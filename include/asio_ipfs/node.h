#pragma once

#include <string>
#include <functional>
#include <memory>
#include <boost/asio/steady_timer.hpp>
#include <boost/asio/io_service.hpp>

namespace asio_ipfs {

struct node_impl;

class node {
    using Timer = boost::asio::steady_timer;
    using Cancel = std::function<void()>;

    template<class Token, class... Ret>
    using Handler = typename boost::asio::handler_type
                        < Token
                        , void(boost::system::error_code, Ret...)
                        >::type;

    template<class Token, class... Ret>
    using Result = typename boost::asio::async_result<Handler<Token, Ret...>>;

public:
    static const uint32_t CID_SIZE = 46;

public:
    // This constructor may do repository initialization disk IO and as such
    // may block for a second or more. If that is undesired, use the static
    // async `node::build` function instead.
    node(boost::asio::io_service&, const std::string& repo_path);

    node(node&&) = default;
    node& operator=(node&&) = default;

    node(const node&) = delete;
    node& operator=(const node&) = delete;

    template<class Token>
    static
    typename Result<Token, std::unique_ptr<node>>::type
    build(boost::asio::io_service&, const std::string& repo_path, Token&&);

    // Returns this node's IPFS ID
    std::string id() const;

    template<class Token>
    typename Result<Token, std::string>::type
    add(const uint8_t* data, size_t size, Token&&);

    template<class Token>
    typename Result<Token, std::string>::type
    add(const std::string&, Token&&); // Convenience function.

    template<class Token>
    typename Result<Token, std::string>::type
    add(const uint8_t* data, size_t size, Cancel&, Token&&);

    template<class Token>
    typename Result<Token, std::string>::type
    add(const std::string&, Cancel&, Token&&); // Convenience function.

    template<class Token>
    typename Result<Token, std::string>::type
    cat(const std::string& cid, Token&&);

    template<class Token>
    typename Result<Token, std::string>::type
    cat(const std::string& cid, Cancel&, Token&&);

    template<class Token>
    void
    publish(const std::string& cid, Timer::duration, Token&&);

    template<class Token>
    void
    publish(const std::string& cid, Timer::duration, Cancel&, Token&&);

    template<class Token>
    typename Result<Token, std::string>::type
    resolve(const std::string& node_id, Token&&);

    template<class Token>
    typename Result<Token, std::string>::type
    resolve(const std::string& node_id, Cancel&, Token&&);

    template<class Token>
    void
    pin(const std::string& cid, Token&&);

    template<class Token>
    void
    pin(const std::string& cid, Cancel&, Token&&);

    template<class Token>
    void
    unpin(const std::string& cid, Token&&);

    template<class Token>
    void
    unpin(const std::string& cid, Cancel&, Token&&);

    boost::asio::io_service& get_io_service();

    ~node();

private:
    node(std::shared_ptr<node_impl>);

    Cancel make_cancel_fn(uint64_t cancel_signal_id);

private:
    static
    void build_( boost::asio::io_service& ios
               , const std::string& repo_path
               , std::function<void( const boost::system::error_code&
                                   , std::unique_ptr<node>)>);

    void add_( const uint8_t* data, size_t size
             , std::function<void(boost::system::error_code, std::string)>);

    void cat_( const std::string& cid
             , Cancel&
             , std::function<void(boost::system::error_code, std::string)>);

    void publish_( const std::string& cid
                 , Timer::duration
                 , Cancel&
                 , std::function<void(boost::system::error_code)>);

    void resolve_( const std::string& ipns_id
                 , Cancel&
                 , std::function<void(boost::system::error_code, std::string)>);

    void pin_( const std::string& cid
             , Cancel&
             , std::function<void(boost::system::error_code)>);

    void unpin_( const std::string& cid
               , Cancel&
               , std::function<void(boost::system::error_code)>);

private:
    std::shared_ptr<node_impl> _impl;
};

template<class Token>
inline
typename node::Result<Token, std::unique_ptr<node>>::type
node::build( boost::asio::io_service& ios
           , const std::string& repo_path
           , Token&& token)
{
    using BackendP = std::unique_ptr<node>;
    Handler<Token, BackendP> handler(std::forward<Token>(token));
    Result<Token, BackendP> result(handler);
    build_(ios, repo_path, std::move(handler));
    return result.get();
}

template<class Token>
inline
typename node::Result<Token, std::string>::type
node::add(const uint8_t* data, size_t size, Token&& token)
{
    Handler<Token, std::string> handler(std::forward<Token>(token));
    Result<Token, std::string> result(handler);
    add_(data, size, std::move(handler));
    return result.get();
}

template<class Token>
inline
typename node::Result<Token, std::string>::type
node::add(const std::string& data, Token&& token)
{
    Handler<Token, std::string> handler(std::forward<Token>(token));
    Result<Token, std::string> result(handler);
    add_( reinterpret_cast<const uint8_t*>(data.c_str())
        , data.size()
        , std::move(handler));
    return result.get();
}

template<class Token>
inline
typename node::Result<Token, std::string>::type
node::cat(const std::string& cid, Token&& token)
{
    Cancel cancel;
    return cat(cid, cancel, std::forward<Token>(token));
}

template<class Token>
inline
typename node::Result<Token, std::string>::type
node::cat(const std::string& cid, Cancel& cancel, Token&& token)
{
    Handler<Token, std::string> handler(std::forward<Token>(token));
    Result<Token, std::string> result(handler);
    cat_(cid, cancel, std::move(handler));
    return result.get();
}

template<class Token>
inline
void
node::publish(const std::string& cid, Timer::duration d, Token&& token)
{
    Cancel cancel;
    return publish(cid, d, cancel, std::forward<Token>(token));
}

template<class Token>
inline
void
node::publish(const std::string& cid, Timer::duration d, Cancel& cancel, Token&& token)
{
    Handler<Token> handler(std::forward<Token>(token));
    Result<Token> result(handler);
    publish_(cid, d, cancel, std::move(handler));
    return result.get();
}

template<class Token>
inline
typename node::Result<Token, std::string>::type
node::resolve(const std::string& ipns_id, Token&& token)
{
    Cancel cancel;
    return resolve(ipns_id, cancel, std::forward<Token>(token));
}

template<class Token>
inline
typename node::Result<Token, std::string>::type
node::resolve(const std::string& ipns_id, Cancel& cancel, Token&& token)
{
    Handler<Token, std::string> handler(std::forward<Token>(token));
    Result<Token, std::string> result(handler);
    resolve_(ipns_id, cancel, std::move(handler));
    return result.get();
}

template<class Token>
inline
void
node::pin(const std::string& cid, Token&& token)
{
    Cancel cancel;
    return pin(cid, cancel, std::forward<Token>(token));
}

template<class Token>
inline
void
node::pin(const std::string& cid, Cancel& cancel, Token&& token)
{
    Handler<Token> handler(std::forward<Token>(token));
    Result<Token> result(handler);
    pin_(cid, cancel, std::move(handler));
    return result.get();
}

template<class Token>
inline
void
node::unpin(const std::string& cid, Token&& token)
{
    Cancel cancel;
    return unpin(cid, cancel, std::forward<Token>(token));
}

template<class Token>
inline
void
node::unpin(const std::string& cid, Cancel& cancel, Token&& token)
{
    Handler<Token> handler(std::forward<Token>(token));
    Result<Token> result(handler);
    unpin_(cid, cancel, std::move(handler));
    return result.get();
}

} // namespace
