#include <asio_ipfs/error.h>

boost::system::error_code
asio_ipfs::error::make_error_code(::asio_ipfs::error::ipfs_error e)
{
    static ipfs_category c;
    return boost::system::error_code(static_cast<int>(e.error_number), c);
}

boost::system::error_code
asio_ipfs::error::make_error_code(::asio_ipfs::error::error_t e)
{
    static asio_ipfs_category c;
    return boost::system::error_code(static_cast<int>(e), c);
}
