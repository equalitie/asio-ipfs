cmake_minimum_required (VERSION 3.5)
################################################################################

# NOTE: from_iso_extended_string has been introduced in Boost 1.62
set(BOOST_VERSION 1.62)
include(ExternalProject)

option(ASIO_IPFS_WITH_EXAMPLE_BINARIES "Build with example binaries" ON)

################################################################################
project(asio-ipfs)

find_package(Threads)
find_package(Boost ${BOOST_VERSION} REQUIRED COMPONENTS
    coroutine
    system
)

if(NOT TARGET golang)
    set(GOROOT "${CMAKE_CURRENT_BINARY_DIR}/golang")
    externalproject_add(golang
        URL https://dl.google.com/go/go1.12.3.linux-amd64.tar.gz
        URL_MD5 eac797050ce084d444a49e8d68ad13b7
        CONFIGURE_COMMAND ""
        BUILD_COMMAND ""
        INSTALL_COMMAND ""
        SOURCE_DIR ${GOROOT}
    )
endif()

# Convert system name into GOOS.
if("${CMAKE_SYSTEM_NAME}" STREQUAL "Linux")
    set(GOOS "linux")
elseif("${CMAKE_SYSTEM_NAME}" STREQUAL "Android")
    set(GOOS "android")
else()
    message(FATAL_ERROR "unsupported system name ${CMAKE_SYSTEM_NAME}")
endif()

# Convert system processor into GOARCH (and maybe GOARM).
if("${CMAKE_SYSTEM_PROCESSOR}" STREQUAL "x86_64")
    set(GOARCH "amd64")
elseif("${CMAKE_SYSTEM_PROCESSOR}" STREQUAL "i686")
    set(GOARCH "386")
elseif("${CMAKE_SYSTEM_PROCESSOR}" STREQUAL "aarch64")
    set(GOARCH "arm64")
elseif("${CMAKE_SYSTEM_PROCESSOR}" STREQUAL "armv7-a")
    set(GOARCH "arm")
    set(GOARM "7")
elseif("${CMAKE_SYSTEM_PROCESSOR}" MATCHES "^arm.*")
    set(GOARCH "arm")
    set(GOARM "5")
else()
    message(FATAL_ERROR "unsupported system processor ${CMAKE_SYSTEM_PROCESSOR}")
endif()

################################################################################

set(GOPATH "${CMAKE_CURRENT_BINARY_DIR}/go-workspace")

# TODO: Temporarily disabled version that takes go-ipfs from
#       official releases due to RC1 candidates not being there.
#externalproject_add(go-ipfs
#    URL https://dist.ipfs.io/go-ipfs/v0.4.19/go-ipfs-source.tar.gz
#    URL_HASH SHA512=89d32fc560683b5bd4f67ae22859d57865ec364e28f065e81bd2618412dccbcc9c5a395f5f297b3ce4c9f6112560ea209e4b2a794c9b0d759b2980569b7ccf82
#    CONFIGURE_COMMAND ""
#    BUILD_COMMAND mkdir -p ${GOPATH}/src/github.com/ipfs
#               && rsync -r ${CMAKE_CURRENT_BINARY_DIR}/go-ipfs-prefix/src/go-ipfs ${GOPATH}/src/github.com/ipfs --exclude=vendor/gx
#               && rsync -r ${CMAKE_CURRENT_BINARY_DIR}/go-ipfs-prefix/src/go-ipfs/vendor/gx ${GOPATH}/src
#    INSTALL_COMMAND ""
#)

set(BINDINGS_DIR "${CMAKE_CURRENT_BINARY_DIR}/ipfs_bindings")
set(BINDINGS_HEADER  "${BINDINGS_DIR}/ipfs_bindings.h")
set(BINDINGS_LIBRARY "${BINDINGS_DIR}/libipfs_bindings.so")
set(BINDINGS_OUTPUT ${BINDINGS_HEADER} ${BINDINGS_LIBRARY})

add_custom_command(
    OUTPUT ${BINDINGS_OUTPUT}
    DEPENDS ${CMAKE_CURRENT_SOURCE_DIR}/include/asio_ipfs/ipfs_error_codes.h
            ${CMAKE_CURRENT_SOURCE_DIR}/src/ipfs_bindings/ipfs_bindings.go
            golang
    COMMAND mkdir -p ${BINDINGS_DIR}
         && export PATH=${GOROOT}/bin:$ENV{PATH}
         && export GOROOT=${GOROOT}
         && export GOPATH=${GOPATH}
         && export GOOS=${GOOS}
         && export GOARCH=${GOARCH}
         && export GOARM=${GOARM}
         && export CGO_ENABLED=1
         && export CC=${CMAKE_C_COMPILER}
         && export GO111MODULE=on
         && export GOPROXY='https://proxy.golang.org' # https://github.com/ipfs/go-ipfs/issues/6460#issuecomment-504937222
         && mkdir -p ${GOPATH}/src/github.com/equalitie/asio-ipfs
         && cd ${GOPATH}/src/github.com/equalitie/asio-ipfs
         && rsync -r ${CMAKE_CURRENT_SOURCE_DIR}/include ${CMAKE_CURRENT_SOURCE_DIR}/src .
         && cp ${CMAKE_CURRENT_SOURCE_DIR}/go.mod ./
         && go build -ldflags '-extldflags -Wl,-soname,libipfs_bindings.so'
                     --buildmode=c-shared
                     -o ${BINDINGS_DIR}/ipfs_bindings.so
                     ./src/ipfs_bindings
         && chmod u+w -R ${GOPATH} # Modules cache is ro by default which CI doesn't cope with; https://github.com/golang/go/issues/27161
         && mv ${BINDINGS_DIR}/ipfs_bindings.so ${BINDINGS_DIR}/libipfs_bindings.so
)
add_custom_target(ipfs-bindings DEPENDS ${BINDINGS_OUTPUT})



################################################################################
file(GLOB sources "./src/*.cpp")
add_library(asio-ipfs STATIC ${sources})
set_target_properties(asio-ipfs PROPERTIES LINKER_LANGUAGE CXX)
add_dependencies(asio-ipfs ipfs-bindings)
target_include_directories(asio-ipfs
    PRIVATE ${BINDINGS_DIR}
    PUBLIC "include"
)
target_compile_definitions(asio-ipfs
    PUBLIC
        # This is a static library, so don't include asio. Users need to link
        # libboost_asio either statically or as a shared library.
        -DBOOST_ASIO_SEPARATE_COMPILATION
        # For some reason we need to define both of these
        -DBOOST_COROUTINES_NO_DEPRECATION_WARNING
        -DBOOST_COROUTINE_NO_DEPRECATION_WARNING
)
target_compile_options(asio-ipfs
    PUBLIC -std=c++14
)
target_link_libraries(asio-ipfs
    PRIVATE ${BINDINGS_LIBRARY}
    PUBLIC Boost::boost
    INTERFACE ${Boost_LIBRARIES}
)


################################################################################
if(ASIO_IPFS_WITH_EXAMPLE_BINARIES)
################################################################################
    find_package(Boost ${BOOST_VERSION} REQUIRED COMPONENTS
        program_options
    )


    # The static library asio-ipfs requires a separately compiled asio,
    # so supply one for the tests and examples.
    add_library(asio-ipfs_static_asio STATIC "lib/asio.cpp")
    target_link_libraries(asio-ipfs_static_asio
        PUBLIC
            Boost::boost
        INTERFACE
            ${Boost_LIBRARIES}
            Threads::Threads
    )
    target_compile_definitions(asio-ipfs_static_asio
        PUBLIC -DBOOST_ASIO_SEPARATE_COMPILATION
    )


    add_executable(ipfs-example "example/ipfs.cpp")
    target_link_libraries(ipfs-example
        asio-ipfs
        asio-ipfs_static_asio
    )
endif() # ASIO_IPFS_WITH_EXAMPLE_BINARIES
