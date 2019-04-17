#include <ipfs_bindings.h>
#include <asio_ipfs/error.h>
#include <assert.h>
#include <experimental/tuple>
#include <boost/intrusive/list.hpp>
#include <boost/optional.hpp>

#include <asio_ipfs.h>

using namespace asio_ipfs;
using namespace std;
namespace asio = boost::asio;
namespace sys  = boost::system;
namespace intr = boost::intrusive;

template<class F> struct Defer { F f; ~Defer() { f(); } };
template<class F> Defer<F> defer(F&& f) { return Defer<F>{forward<F>(f)}; }

struct HandleBase : public intr::list_base_hook
                            <intr::link_mode<intr::auto_unlink>> {
    virtual void cancel() = 0;
    virtual ~HandleBase() { }
};

struct asio_ipfs::node_impl {
    uint64_t ipfs_handle;
    asio::io_service& ios;
    intr::list<HandleBase, intr::constant_time_size<false>> handles;

    node_impl(asio::io_service& ios)
        : ios(ios)
    {}
};



template<class... As>
struct Handle : public HandleBase {
    asio::io_service& ios;
    uint64_t ipfs_handle;
    function<void(sys::error_code, As&&...)> cb;
    function<void()>* cancel_fn;
    function<void()> destructor_cancel_fn;
    boost::optional<uint64_t> cancel_signal_id;
    asio::io_service::work work;
    unsigned job_count = 1;

    Handle( node_impl* impl
          , boost::optional<uint64_t> cancel_signal_id_
          , function<void()>* cancel_fn_
          , function<void(sys::error_code, As&&...)> cb_)
        : ios(impl->ios)
        , ipfs_handle(impl->ipfs_handle)
        , cancel_fn(cancel_fn_ ? cancel_fn_ : &destructor_cancel_fn)
        , cancel_signal_id(cancel_signal_id_)
        , work(asio::io_service::work(ios))
    {
        impl->handles.push_back(*this);

        cb = [this, cb_ = std::move(cb_)] (sys::error_code ec, As... args) {
            (*cancel_fn) = []{};
            if (cancel_signal_id) {
                go_asio_ipfs_cancellation_free(ipfs_handle, *cancel_signal_id);
            }
            // We need to unlink here, othersize the callback could invoke the
            // destructor, which would in turn call `cancel` and expect that it
            // gets unlinked. But we just set the `cancel_fn` to do nothing
            // above, so the destructor ends up in an infinite loop.
            unlink();
            std::experimental::apply(cb_, make_tuple(ec, std::move(args)...));
        };

        *cancel_fn = [this] {
            unlink();
            if (cancel_signal_id) {
                go_asio_ipfs_cancel(ipfs_handle, *cancel_signal_id);
            }

            assert(cb);
            assert(job_count);
            ++job_count;

            ios.post([this, callback = std::move(cb)] {
                auto on_exit = defer([&] { if (!--job_count) delete(this); });

                tuple<sys::error_code, As...> args;
                std::get<0>(args) = asio::error::operation_aborted;
                std::experimental::apply(callback, std::move(args));
            });

            (*cancel_fn) = []{};
        };

        /*
         * Exactly one of cb and *cancel_fn is ever called, in the asio thread.
         */
    }

    /*
     * This function is always called, in a go thread. If the Handle was
     * cancelled, self->cb is empty.
     */
    static void call(int err, void* arg, As... args) {
        auto self = reinterpret_cast<Handle*>(arg);
        self->ios.post([
            self,
            full_args = make_tuple(make_error_code(error::ipfs_error{err}), std::move(args)...)
        ] {
            auto on_exit = defer([&] { if (!--self->job_count) delete(self); });

            if (self->cb) {
                std::experimental::apply(self->cb, tuple<sys::error_code, As...>(std::move(full_args)));
            }
        });
    }

    void cancel() override {
        (*cancel_fn)();
    }
};

template<class... As> struct callback_function;

template<> struct callback_function<> {
    static void callback(int err, void* arg) {
        Handle<>::call(err, arg);
    }
};

template<> struct callback_function<std::string> {
    static void callback(int err, const char* data, size_t size, void* arg) {
        Handle<std::string>::call(err, arg, std::string(data, data + size));
    }
};

template<class... CbAs, class F, class... As>
void call_ipfs(
    node_impl* node,
    std::function<void()>* cancel,
    std::function<void(sys::error_code, CbAs...)> callback,
    F ipfs_function,
    As... args
) {
    uint64_t cancel_signal_id = go_asio_ipfs_cancellation_allocate(node->ipfs_handle);

    ipfs_function(
        node->ipfs_handle,
        cancel_signal_id,
        args...,
        (void*) &callback_function<CbAs...>::callback,
        (void*) (new Handle<CbAs...>{ node, cancel_signal_id, cancel, std::move(callback) })
    );
}

template<class... CbAs, class F, class... As>
void call_ipfs_nocancel(
    node_impl* node,
    std::function<void()>* cancel,
    std::function<void(sys::error_code, CbAs...)> callback,
    F ipfs_function,
    As... args
) {
    ipfs_function(
        node->ipfs_handle,
        args...,
        (void*) &callback_function<CbAs...>::callback,
        (void*) (new Handle<CbAs...>{ node, boost::none, cancel, std::move(callback) })
    );
}



node::node(asio::io_service& ios, bool online, const string& repo_path)
{
    uint64_t ipfs_handle = go_asio_ipfs_allocate();
    int ec = go_asio_ipfs_start_blocking(ipfs_handle, online, (char*) repo_path.data());

    if (ec != IPFS_SUCCESS) {
        go_asio_ipfs_free(ipfs_handle);
        throw std::runtime_error("node: Failed to start IPFS");
    }

    _impl = make_unique<node_impl>(ios);
    _impl->ipfs_handle = ipfs_handle;
}

void node::build_( asio::io_service& ios
                 , bool online
                 , const string& repo_path
                 , Cancel* cancel
                 , function<void( const sys::error_code& ec
                                , unique_ptr<node>)> cb)
{
    /*
     * This cannot be a unique_ptr, because std::function wants to be
     * CopyConstructible for some reason.
     */
    auto impl = new node_impl(ios);
    impl->ipfs_handle = go_asio_ipfs_allocate();

    std::function<void(sys::error_code)> cb_ = [cb = move(cb), impl] (sys::error_code ec) {
        if (ec) {
            go_asio_ipfs_free(impl->ipfs_handle);
            delete impl;
            cb(ec, nullptr);
        } else {
            std::unique_ptr<node> node_(new node);
            node_->_impl = unique_ptr<node_impl>(impl);
            cb(ec, std::move(node_));
        }
    };

    call_ipfs_nocancel(impl, cancel, cb_, go_asio_ipfs_start_async, online, (char*) repo_path.data());
}

node::node() = default;
node::node(node&&) = default;
node& node::operator=(node&&) = default;


string node::id() const {
    char* cid = go_asio_ipfs_node_id(_impl->ipfs_handle);
    string ret(cid);
    free(cid);
    return ret;
}

void node::publish_( const string& cid
                   , Timer::duration d
                   , Cancel* cancel
                   , std::function<void(sys::error_code)> cb)
{
    assert(cid.size() == CID_SIZE);

    call_ipfs(_impl.get(), cancel, cb, go_asio_ipfs_publish, (char*) cid.data(), std::chrono::duration_cast<std::chrono::seconds>(d).count());
}

void node::resolve_( const string& node_id
                   , Cancel* cancel
                   , function<void(sys::error_code, string)> cb)
{
    call_ipfs(_impl.get(), cancel, cb, go_asio_ipfs_resolve, (char*) node_id.data());
}

void node::add_( const uint8_t* data
               , size_t size
               , Cancel* cancel
               , function<void(sys::error_code, string)> cb)
{
    call_ipfs_nocancel(_impl.get(), cancel, cb, go_asio_ipfs_add, (void*) data, size, false);
}

void node::calculate_cid_( const string_view data
                         , Cancel* cancel
                         , function<void(sys::error_code, string)> cb)
{
    const char* d = data.data();
    size_t s = data.size();
    call_ipfs_nocancel(_impl.get(), cancel, cb, go_asio_ipfs_add, (void*) d, s, true);
}

void node::cat_( string_view cid
               , Cancel* cancel
               , function<void(sys::error_code, string)> cb)
{
    assert(cid.size() == CID_SIZE);

    call_ipfs(_impl.get(), cancel, cb, go_asio_ipfs_cat, (char*) cid.data());
}

void node::pin_( const string& cid
               , Cancel* cancel
               , std::function<void(sys::error_code)> cb)
{
    assert(cid.size() == CID_SIZE);

    call_ipfs(_impl.get(), cancel, cb, go_asio_ipfs_pin, (char*) cid.data());
}

void node::unpin_( const string& cid
                 , Cancel* cancel
                 , std::function<void(sys::error_code)> cb)
{
    assert(cid.size() == CID_SIZE);

    call_ipfs(_impl.get(), cancel, cb, go_asio_ipfs_unpin, (char*) cid.data());
}

boost::asio::io_service& node::get_io_service()
{
    return _impl->ios;
}

node::~node()
{
    if (_impl) {
        // Make sure all handlers get completed.
        while (!_impl->handles.empty()) {
            auto& e = _impl->handles.front();
            e.cancel();
            /*
             * The handle will unlink itself in cancel(),
             * so there is no need to pop_front().
             */
        }

        go_asio_ipfs_free(_impl->ipfs_handle);
    }
}
