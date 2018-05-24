#pragma once

#include <string>
#include <boost/system/error_code.hpp>

#include <asio_ipfs/ipfs_error_codes.h>

namespace asio_ipfs { namespace error {

    struct ipfs_error {
        int error_number;
    };
    
    enum error_t {
        db_download_failed = 1, // Start with > 0, because 0 means success.,
        invalid_db_format,
        malformed_db_entry,
        missing_ipfs_link,
    };
    
    struct ipfs_category : public boost::system::error_category
    {
        const char* name() const noexcept override
        {
            return "ipfs_errors";
        }
    
        std::string message(int e) const override
        {
            switch (e) {
                case IPFS_SUCCESS:
                    return "success";
                case IPFS_RESOLVE_FAILED:
                    return "failed to resolve IPNS entry";
                case IPFS_FAILED_TO_CREATE_REPO:
                    return "failed to create IPFS repository";
                case IPFS_ADD_FAILED:
                    return "failed to add data";
                case IPFS_CAT_FAILED:
                    return "failed to get data reader";
                case IPFS_READ_FAILED:
                    return "failed to read data";
                case IPFS_PUBLISH_FAILED:
                    return "failed to publish CID";
                case IPFS_PIN_FAILED:
                    return "failed to pin";
                case IPFS_UNPIN_FAILED:
                    return "failed to unpin";
                default:
                    return "unknown ipfs error";
            }
        }
    };
    
    struct asio_ipfs_category : public boost::system::error_category
    {
        const char* name() const noexcept override
        {
            return "asio_ipfs_errors";
        }
    
        std::string message(int e) const override
        {
            switch (e) {
                case error::db_download_failed:
                    return "database download failed";
                case error::invalid_db_format:
                    return "invalid database format";
                case error::malformed_db_entry:
                    return "malformed database entry";
                case error::missing_ipfs_link:
                    return "missing IPFS link to content";
                default:
                    return "unknown asio_ipfs error";
            }
        }
    };
    
    boost::system::error_code
    make_error_code(::asio_ipfs::error::ipfs_error);
    
    boost::system::error_code
    make_error_code(::asio_ipfs::error::error_t);

}} // asio_ipfs::error namespace

namespace boost { namespace system {

    template<>
    struct is_error_code_enum<::asio_ipfs::error::error_t>
        : public std::true_type {};


}} // boost::system namespace
