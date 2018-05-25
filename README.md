[![CircleCI](https://circleci.com/gh/equalitie/asio-ipfs/tree/master.svg?style=shield)](https://circleci.com/gh/equalitie/asio-ipfs/tree/master)

# Asio.IPFS

A C++ Boost.Asio wrapper library over go-ipfs.

## Features

* Boost.Asio based event loop
* Supports callbacks, futures and coroutines
* Cmake automatically downloads `golang` and `go-ipfs` + its dependencies

## Caveats/TODOs

* Destroying `asio_ipfs::node` will cancel all peding IPFS async operations,
  but at the moment they can't be cancelled individually.
* The `asio::io_service` can run in only one thread.
* Only a basic subset of IPFS operations are currently supported, have a look
  at `asio_ipfs/node.h` for details.
* The `node::cat` operation returns the content as a whole (this
  is OK for small contents, but some kind of stream would be preferred
  for big ones)

## Requirements

To be able to use the Asio.IPFS in platforms like Android, where running IPFS as an
independent daemon is not a possibility, the wrapper needs to embed IPFS by linking
directly with its Go code.  Thus the source of go-ipfs is needed to build the main
glue between C++ and IPFS.  Building that source requires a recent version of Go.  To
avoid extra system dependencies, the build process automatically downloads the Go
system and builds IPFS itself.

In summary, the minimum build dependencies are:

* `cmake` 3.5+
* `g++` capable of C++14 (`clang` not tested, but there's no reason to thing it
  wouldn't work)
* The [Boost library](http://www.boost.org/) v1.62 or higher

For Debian, this translates to the following packages:

* `build-essential`
* `cmake`
* `curl`
* `libboost-dev`
* `libboost-system-dev`
* `libboost-coroutine-dev` (only when it's to be used with coroutines)
* `libboost-program-options-dev` (only for the examples)

The build process is able to compile the Asio.IPFS to different platforms with the
help of a properly configured cross-compilation environment.  If you actually intend
to cross-compile you will need proper C/C++ cross-compiler packages, Boost libraries
for the target system and a toolchain file for CMake to use them.

To the date, the build process has only been tested on 64-bit GNU/Linux platforms
and ARM based Androids.

## Building

    $ cd <PROJECT ROOT>
    $ mkdir build
    $ cd build
    $ cmake ..
    $ make

On success, the _build_ directory shall contain the _libipfs-bindings.so_
library, _libasio-ipfs.a_ archive and one example programs _ipfs-example_.

To cross-compile to another system, you may either create a different `build`
directory, or reuse the same directory and just remove the `CMakeCache.txt` file (thus
you can reuse some downloads and build tools).  Just remember to point CMake to the
proper toolchain file.  For the previous Raspbian example:

    cmake -DCMAKE_TOOLCHAIN_FILE=/path/to/toolchain-linux-armhf-gcc6.cmake ..
    make

### Linux cross-compilation example

For building binaries in a Debian Strech machine which are able to run on _Raspbian
Stretch_ on the Raspberry Pi:

  - Install the `gcc-6-arm-linux-gnueabihf` and `g++-6-arm-linux-gnueabihf` packages.
  - As indicated in <https://wiki.debian.org/Multiarch/HOWTO>, add the new
    architecture with `dpkg --add-architecture armhf` and update your package list.
  - Install the Boost libraries matching the target distribution, with the proper
    architecture suffix:

      - `libboost-system1.62-dev:armhf`
      - `libboost-coroutine1.62-dev:armhf`
      - `libboost-program-options1.62-dev:armhf`

  - Create a toolchain file (e.g. `toolchain-linux-armhf-gcc6.cmake`) containing:

        set(CMAKE_SYSTEM_NAME Linux)
        set(CMAKE_SYSTEM_PROCESSOR armv6l)

        set(CMAKE_C_COMPILER /usr/bin/arm-linux-gnueabihf-gcc-6)
        set(CMAKE_CXX_COMPILER /usr/bin/arm-linux-gnueabihf-g++-6)

### Android cross-compilation example

For building binaries able to run in _Android KitKat and above on ARM_ processors you
will need a Clang/LLVM standalone toolchain created with the Android NDK.  Assuming
that the NDK is under `~/opt/android-ndk-r15c`, you may run:

    $ ~/opt/android-ndk-r15c/build/tools/make-standalone-toolchain.sh \
      --platform=android-19 --arch=arm --stl=libc++ \
      --install-dir=$HOME/opt/ndk-android19-arm-libcpp

You will also need to build the Boost libraries for this platform.  You may use
[Boost for Android](https://github.com/dec1/Boost-for-Android).  Assuming that Boost
source is in `~/src/boost/<BOOST_VERSION>`, edit `doIt.sh` and:

  - set `BOOST_SRC_DIR` to `$HOME/src/boost`
  - set `BOOST_VERSION` to the `<BOOST_VERSION>` above
  - set `GOOGLE_DIR` to `$HOME/opt/android-ndk-r15c`
  - modify `build-boost.sh` arguments, set `--version=$BOOST_VERSION`,
    `--stdlibs="llvm-3.5"`, `--linkage="shared"` and `--abis` to the desired
    architectures (`armeabi-v7a` in our example)

Create the `llvm-3.5` link as indicated in Boost for Android's readme and run
`./doIt.sh` to build the Boost libraries.  This will create the directory
`build/boost/<BOOST_VERSION>`.

After the previous steps you can use a CMake toolchain file like the following one:

    set(CMAKE_SYSTEM_NAME Android)
    set(CMAKE_SYSTEM_VERSION 19)
    set(CMAKE_ANDROID_ARCH_ABI armeabi-v7a)
    set(CMAKE_ANDROID_STANDALONE_TOOLCHAIN $ENV{HOME}/opt/ndk-android19-arm-libcpp)

    set(BOOST_INCLUDEDIR /path/to/Boost-for-Android/build/boost/<BOOST_VERSION>/include)
    set(BOOST_LIBRARYDIR /path/to/Boost-for-Android/build/boost/<BOOST_VERSION>/libs/${CMAKE_ANDROID_ARCH_ABI}/llvm-3.5)
