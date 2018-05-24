#include <iostream>
#include <asio_ipfs.h>
#include <boost/program_options.hpp>
#include <boost/asio/io_service.hpp>
#include <boost/asio/steady_timer.hpp>

namespace asio = boost::asio;

using std::string;
using std::cout;
using std::cerr;
using std::endl;
namespace chrono = std::chrono;

void sleep_forever(asio::io_service& ios, asio::yield_context yield)
{
    asio::steady_timer timer(ios);

    while (true) {
        timer.expires_from_now(chrono::seconds(1));
        timer.async_wait(yield);
    }
}

int main(int argc, const char** argv)
{
    namespace po = boost::program_options;

    po::options_description desc("Options");

    desc.add_options()
        ("help", "Produce this help message")
        ("repo,r", po::value<string>(),
         "Path to the IPFS repository (must be set)")
        ("add", po::value<string>(), "Perform `ipfs add` operation")
        ("cat", po::value<string>(), "Perform `ipfs cat` operation")
        ;

    po::variables_map vm;
    po::store(po::parse_command_line(argc, argv, desc), vm);
    po::notify(vm); 

    if (vm.count("help")) {
        cout << desc << endl;
        return 0;
    }

    if (!vm.count("repo")) {
        cerr << "The 'repo' parameter must be set" << endl;
        cerr << desc << endl;
        return 1;
    }

    string repo = vm["repo"].as<string>();

    asio::io_service ios;

    cout << "Starting event loop, press Ctrl-C to exit." << endl;

    asio::spawn(ios, [&](asio::yield_context yield) {
            auto n = asio_ipfs::node::build(ios, repo, yield);

            if (vm.count("add")) {
                string cid = n->add(vm["add"].as<string>(), yield);

                cout << "CID: " << cid << endl;

                // Prevent the app from exiting so that other nodes can
                // download the content from us.
                sleep_forever(ios, yield);
            }
            else if (vm.count("cat")) {
                string content = n->cat(vm["cat"].as<string>(), yield);

                cout << "Content: " << content << endl;
            }
        });

    ios.run();

    return 0;
}
