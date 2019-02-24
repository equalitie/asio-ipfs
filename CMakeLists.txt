cmake_minimum_required (VERSION 3.5)
################################################################################

# NOTE: from_iso_extended_string has been introduced in Boost 1.62
set(BOOST_VERSION 1.62)
include(ExternalProject)

################################################################################
project(asio-ipfs)

find_package(Threads)
find_package(Boost ${BOOST_VERSION} REQUIRED COMPONENTS
    program_options
    coroutine
    context
    thread
    system)

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
elseif("${CMAKE_SYSTEM_PROCESSOR}" STREQUAL "aarch64")
    set(GOARCH "arm64")
elseif("${CMAKE_SYSTEM_PROCESSOR}" STREQUAL "armv7-a")
    set(GOARCH "arm")
    set(GOARM "7")
elseif("${CMAKE_SYSTEM_PROCESSOR}" MATCHES "^arm.*")
    set(GOARCH "arm")
    set(GOARM "6")
else()
    message(FATAL_ERROR "unsupported system processor ${CMAKE_SYSTEM_PROCESSOR}")
endif()

# Build target tag with the Android API version if relevant.
if("${CMAKE_SYSTEM_NAME}" STREQUAL "Android")
    set(TARGET "${CMAKE_SYSTEM_NAME}${CMAKE_SYSTEM_VERSION}--${CMAKE_SYSTEM_PROCESSOR}")
else()
    set(TARGET "${CMAKE_SYSTEM_NAME}--${CMAKE_SYSTEM_PROCESSOR}")
endif()

################################################################################
option(ASIO_IPFS_WITH_EXAMPLE_BINARIES "Build with example binaries" ON)

add_definitions(-DBOOST_COROUTINE_NO_DEPRECATION_WARNING
                -DBOOST_COROUTINES_NO_DEPRECATION_WARNING)

################################################################################
# TODO(peterj): Address sanitizer seems to cause segmentation faults on exit
#               from main(?).
#set(SANITIZE "-fsanitize=address")
################################################################################

# Android does not support POSIX file:
# <https://github.com/boostorg/beast/issues/800>.
if("${CMAKE_SYSTEM_NAME}" STREQUAL "Android")
    set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} -DBOOST_BEAST_NO_POSIX_FILE=0")
endif()
set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} -std=c++14 -pthread -Wall -ggdb ${SANITIZE}")

################################################################################

set(GOROOT "${CMAKE_CURRENT_BINARY_DIR}/golang")

################################################################################

set(PATCH "${CMAKE_CURRENT_SOURCE_DIR}/patches/0001-Throttle.patch")
set(GOPATH "${CMAKE_CURRENT_BINARY_DIR}/go-workspace")

externalproject_add(go-ipfs
    URL /gnu/store/rx6zymg83kyqd8hnvhxflb9xv0hjfa0b-go-ipfs-source.tar.gz
    #PATCH_COMMAND cd ${CMAKE_CURRENT_BINARY_DIR}/go-ipfs-prefix/src/go-ipfs/vendor
    #           && patch -p0 -b < ${PATCH}
    CONFIGURE_COMMAND ""
    BUILD_COMMAND mkdir -p ${GOPATH}/src/github.com/ipfs
               && rsync -r ${CMAKE_CURRENT_BINARY_DIR}/go-ipfs-prefix/src/go-ipfs ${GOPATH}/src/github.com/ipfs --exclude=vendor/gx
               && rsync -r ${CMAKE_CURRENT_BINARY_DIR}/go-ipfs-prefix/src/go-ipfs/vendor/gx ${GOPATH}/src
    INSTALL_COMMAND ""
)

set(BINDING_DIR "${CMAKE_CURRENT_BINARY_DIR}/ipfs_bindings")
set(BINDINGS_HEADER  "${BINDING_DIR}/ipfs_bindings.h")
set(BINDINGS_LIBRARY "${BINDING_DIR}/libipfs_bindings.so")
set(BINDINGS_OUTPUT ${BINDINGS_HEADER} ${BINDINGS_LIBRARY})

add_custom_command(
    OUTPUT ${BINDINGS_OUTPUT}
    DEPENDS ${CMAKE_CURRENT_SOURCE_DIR}/include/asio_ipfs/ipfs_error_codes.h
            ${CMAKE_CURRENT_SOURCE_DIR}/src/ipfs_bindings/ipfs_bindings.go
            go-ipfs
    COMMAND mkdir -p ${BINDING_DIR}
         && export IPFS_GX_USE_GLOBAL=1
         && export GOPATH=${GOPATH}
         && export GOOS=${GOOS}
         && export GOARCH=${GOARCH}
         && export GOARM=${GOARM}
         && export CGO_ENABLED=1
         && export CC=${CMAKE_C_COMPILER}
         && mkdir -p ${GOPATH}/src/github.com/equalitie/asio-ipfs
         && cd ${GOPATH}/src/github.com/equalitie/asio-ipfs
         && rsync -r ${CMAKE_CURRENT_SOURCE_DIR}/include ${CMAKE_CURRENT_SOURCE_DIR}/src .
         && go build -ldflags '-extldflags -Wl,-soname,libipfs_bindings.so'
                     --buildmode=c-shared
                     -o ${BINDING_DIR}/ipfs_bindings.so
                     ./src/ipfs_bindings
         && mv ${BINDING_DIR}/ipfs_bindings.so ${BINDING_DIR}/libipfs_bindings.so
)

add_custom_target(ipfs-bindings-target DEPENDS ${BINDINGS_OUTPUT})
add_library(ipfs-bindings SHARED IMPORTED GLOBAL)
add_dependencies(ipfs-bindings ipfs-bindings-target)
set_target_properties(ipfs-bindings PROPERTIES IMPORTED_LOCATION ${BINDINGS_LIBRARY})

################################################################################
include_directories(
    "${Boost_INCLUDE_DIR}"
    "${BINDING_DIR}"
    "./include")

file(GLOB sources
  "./src/*.h"
  "./src/*.cpp")

add_library(asio-ipfs STATIC ${sources})
set_target_properties(asio-ipfs PROPERTIES LINKER_LANGUAGE CXX)
target_link_libraries(asio-ipfs ipfs-bindings ${Boost_LIBRARIES})

################################################################################
# Only set these variables if this CMakeLists.txt is included from another one.
get_directory_property(hasParent PARENT_DIRECTORY)
if(hasParent)
    set(ASIO_IPFS_INCLUDE_DIR "${CMAKE_CURRENT_SOURCE_DIR}/include" PARENT_SCOPE)
    set(ASIO_IPFS_TARGET "asio-ipfs" PARENT_SCOPE)
    set(ASIO_IPFS_LIBRARIES
        asio-ipfs
        ipfs-bindings
        PARENT_SCOPE)
endif()

################################################################################
if(ASIO_IPFS_WITH_EXAMPLE_BINARIES)
################################################################################
set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} -std=c++14 -pthread -Wall -ggdb ${SANITIZE}")

include_directories(
    "./include")

set(sources "./example/ipfs.cpp")

add_executable(ipfs-example ${sources})
add_dependencies(ipfs-example asio-ipfs)
target_link_libraries(ipfs-example asio-ipfs ipfs-bindings ${Boost_LIBRARIES})

################################################################################
endif() # ASIO_IPFS_WITH_EXAMPLE_BINARIES
################################################################################
install(TARGETS asio-ipfs EXPORT asio-ipfs
        ARCHIVE DESTINATION lib COMPONENT Libraries)
################################################################################
