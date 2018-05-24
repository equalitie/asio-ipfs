// Useful links:
// https://github.com/ipfs/go-ipfs/issues/3060
// https://github.com/ipfs/examples/tree/master/examples

package main

import (
	"fmt"
	"context"
	"os"
	"bytes"
	"sort"
	"unsafe"
	"time"
	"io"
	"strings"
	"io/ioutil"
	core "github.com/ipfs/go-ipfs/core"
	coreapi "github.com/ipfs/go-ipfs/core/coreapi"
	coreiface "github.com/ipfs/go-ipfs/core/coreapi/interface"
	repo "github.com/ipfs/go-ipfs/repo"
	fsrepo "github.com/ipfs/go-ipfs/repo/fsrepo"
	config "github.com/ipfs/go-ipfs/repo/config"
	path "github.com/ipfs/go-ipfs/path"
	"github.com/ipfs/go-ipfs/core/coreunix"

	peer "gx/ipfs/QmZoWKhxUmZ2seW4BzX6fJkNR8hh9PsGModr7q171yq2SS/go-libp2p-peer"
)

// #cgo CFLAGS: -DIN_GO=1 -ggdb -I ${SRCDIR}/../../include
// #cgo android LDFLAGS: -Wl,--unresolved-symbols=ignore-all
//#include <stdlib.h>
//#include <stddef.h>
//#include <stdint.h>
//#include <asio_ipfs/ipfs_error_codes.h>
//
//// Don't export these functions into C or we'll get "unused function" warnings
//// (Or errors saying functions are defined more than once if the're not static).
//
//#if IN_GO
//static void execute_void_cb(void* func, int err, void* arg)
//{
//    ((void(*)(int, void*)) func)(err, arg);
//}
//static void execute_data_cb(void* func, int err, void* data, size_t size, void* arg)
//{
//    ((void(*)(int, char*, size_t, void*)) func)(err, data, size, arg);
//}
//#endif // if IN_GO
import "C"

const (
	nBitsForKeypair = 2048
	repoRoot = "./repo"
	debug = false

	// This next option makes IPNS resolution much faster:
	//
	// https://blog.ipfs.io/34-go-ipfs-0.4.14#ipns-improvements
	//
	// But:
	//
	// 1. It's an experimental feature which may result in inefficient message
	//    flooding when many users are participating. Thus it's disabled by
	//    default. Use only for debugging (if at all).
	//
	// 2. It doesn't help with the first publish and first resolve, though
	//    it seems that once the publisher and resolver found their places
	//    in the DHT, the two operations become almost instant.

	enablePubSubIPNS = true
)

func main() {
}

func doesnt_exist_or_is_empty(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return true
		}
		return false
	}
	defer f.Close()

	_, err = f.Readdirnames(1)
	if err == io.EOF {
		return true
	}
	return false
}

// "/ip4/0.0.0.0/tcp/4001" -> "/ip4/0.0.0.0/tcp/0"
// "/ip6/::/tcp/4001"      -> "/ip6/::/tcp/0"
func setRandomPort(ep string) string {
	parts := strings.Split(ep, "/")
	l := len(parts);
	if l == 0 { return ep }
	parts[l-1] = "0"
	return strings.Join(parts, "/")
}

func openOrCreateRepo(repoRoot string) (repo.Repo, error) {
	if doesnt_exist_or_is_empty(repoRoot) {
		conf, err := config.Init(os.Stdout, nBitsForKeypair)

		if err != nil {
			return nil, err
		}

		// Don't use hardcoded swarm ports (usually 4001), otherwise
		// we wouldn't be able to run multiple IPFS instances on the
		// same PC.
		for i, addr := range conf.Addresses.Swarm {
			conf.Addresses.Swarm[i] = setRandomPort(addr)
		}

		if err := fsrepo.Init(repoRoot, conf); err != nil {
			return nil, err
		}
	}

	return fsrepo.Open(repoRoot)
}

func printSwarmAddrs(node *core.IpfsNode) {
	if !node.OnlineMode() {
		fmt.Println("Swarm not listening, running in offline mode.")
		return
	}
	var addrs []string
	for _, addr := range node.PeerHost.Addrs() {
		addrs = append(addrs, addr.String())
	}
	sort.Sort(sort.StringSlice(addrs))

	for _, addr := range addrs {
		fmt.Printf("Swarm listening on %s\n", addr)
	}
}

type Cache struct {
	node *core.IpfsNode
	api coreiface.CoreAPI
	ctx context.Context
	cancel context.CancelFunc
}

var g Cache

func start_cache(repoRoot string) C.int {
	g.ctx, g.cancel = context.WithCancel(context.Background())

	r, err := openOrCreateRepo(repoRoot);

	if err != nil {
		fmt.Println("err", err);
		return C.IPFS_FAILED_TO_CREATE_REPO
	}

	g.node, err = core.NewNode(g.ctx, &core.BuildCfg{
		Online: true,
		Permanent: true,
		Repo:   r,
		ExtraOpts: map[string]bool{
			"ipnsps": enablePubSubIPNS,
		},
	})

	g.node.SetLocal(false)

	printSwarmAddrs(g.node)

	g.api = coreapi.NewCoreAPI(g.node)

	return C.IPFS_SUCCESS
}

//export go_asio_ipfs_start
func go_asio_ipfs_start(c_repoPath *C.char) C.int {
	repoRoot := C.GoString(c_repoPath)
	return start_cache(repoRoot);
}

//export go_asio_ipfs_async_start
func go_asio_ipfs_async_start(c_repoPath *C.char, fn unsafe.Pointer, fn_arg unsafe.Pointer) {
	repoRoot := C.GoString(c_repoPath)
	go func() {
		err := start_cache(repoRoot);
		C.execute_void_cb(fn, err, fn_arg)
	}()
}

//export go_asio_ipfs_stop
func go_asio_ipfs_stop() {
	g.cancel()
}

//export go_asio_ipfs_resolve
func go_asio_ipfs_resolve(c_ipns_id *C.char, fn unsafe.Pointer, fn_arg unsafe.Pointer) {
	ipns_id := C.GoString(c_ipns_id)

	go func() {
		if debug {
			fmt.Println("go_asio_ipfs_resolve start");
			defer fmt.Println("go_asio_ipfs_resolve end");
		}

		ctx := g.ctx
		n := g.node
		p := path.Path("/ipns/" + ipns_id)

		node, err := core.Resolve(ctx, n.Namesys, n.Resolver, p)

		if err != nil {
			C.execute_data_cb(fn, C.IPFS_RESOLVE_FAILED, nil, C.size_t(0), fn_arg)
			return
		}

		data := []byte(node.Cid().String())
		cdata := C.CBytes(data)
		defer C.free(cdata)

		C.execute_data_cb(fn, C.IPFS_SUCCESS, cdata, C.size_t(len(data)), fn_arg)
	}()
}

// IMPORTANT: The returned value needs to be explicitly `free`d.
//export go_asio_ipfs_ipns_id
func go_asio_ipfs_ipns_id() *C.char {
	pid, err := peer.IDFromPrivateKey(g.node.PrivateKey)

	if err != nil {
		return nil
	}

	cstr := C.CString(pid.Pretty())
	return cstr
}

func publish(ctx context.Context, duration time.Duration, n *core.IpfsNode, cid string) error {
	path, err := path.ParseCidToPath(cid)

	if err != nil {
		fmt.Println("go_asio_ipfs_publish failed to parse cid \"", cid, "\"");
		return err
	}

	k := n.PrivateKey

	eol := time.Now().Add(duration)
	err  = n.Namesys.PublishWithEOL(ctx, k, path, eol)

	if err != nil {
		fmt.Println("go_asio_ipfs_publish failed");
		return err
	}

	return nil
}

//export go_asio_ipfs_publish
func go_asio_ipfs_publish(cid *C.char, seconds C.int64_t, fn unsafe.Pointer, fn_arg unsafe.Pointer) {
	id := C.GoString(cid)

	go func() {
		if debug {
			fmt.Println("go_asio_ipfs_publish start");
			defer fmt.Println("go_asio_ipfs_publish end");
		}

		// https://stackoverflow.com/questions/17573190/how-to-multiply-duration-by-integer
		err := publish(g.ctx, time.Duration(seconds) * time.Second, g.node, id);

		if err != nil {
			C.execute_void_cb(fn, C.IPFS_PUBLISH_FAILED, fn_arg)
			return
		}

		C.execute_void_cb(fn, C.IPFS_SUCCESS, fn_arg)
	}()
}

//export go_asio_ipfs_add
func go_asio_ipfs_add(data unsafe.Pointer, size C.size_t, fn unsafe.Pointer, fn_arg unsafe.Pointer) {
	msg := C.GoBytes(data, C.int(size))

	go func() {
		if debug {
			fmt.Println("go_asio_ipfs_add start");
			defer fmt.Println("go_asio_ipfs_add end");
		}

		cid, err := coreunix.Add(g.node, bytes.NewReader(msg))

		if err != nil {
			fmt.Println("Error: failed to insert content ", err)
			C.execute_data_cb(fn, C.IPFS_ADD_FAILED, nil, C.size_t(0), fn_arg)
			return;
		}

		cdata := C.CBytes([]byte(cid))
		defer C.free(cdata)

		C.execute_data_cb(fn, C.IPFS_SUCCESS, cdata, C.size_t(len(cid)), fn_arg)
	}()
}

//export go_asio_ipfs_cat
func go_asio_ipfs_cat(c_cid *C.char, fn unsafe.Pointer, fn_arg unsafe.Pointer) {
	cid := C.GoString(c_cid)

	go func() {
		if debug {
			fmt.Println("go_asio_ipfs_cat start");
			defer fmt.Println("go_asio_ipfs_cat end");
		}

		reader, err := coreunix.Cat(g.ctx, g.node, cid)

		if err != nil {
			fmt.Println("go_asio_ipfs_cat failed to Cat");
			C.execute_data_cb(fn, C.IPFS_CAT_FAILED, nil, C.size_t(0), fn_arg)
			return
		}

		bytes, err := ioutil.ReadAll(reader)
		if err != nil {
			fmt.Println("go_asio_ipfs_cat failed to read");
			C.execute_data_cb(fn, C.IPFS_READ_FAILED, nil, C.size_t(0), fn_arg)
			return
		}

		cdata := C.CBytes(bytes)
		defer C.free(cdata)

		C.execute_data_cb(fn, C.IPFS_SUCCESS, cdata, C.size_t(len(bytes)), fn_arg)
	}()
}

//export go_asio_ipfs_pin
func go_asio_ipfs_pin(c_cid *C.char, fn unsafe.Pointer, fn_arg unsafe.Pointer) {
	cid := C.GoString(c_cid)

	go func() {
		if debug {
			fmt.Println("go_asio_ipfs_pin start");
			defer fmt.Println("go_asio_ipfs_pin end");
		}

		path, err := coreapi.ParsePath(cid)

		if err != nil {
			fmt.Printf("go_asio_ipfs_pin failed to unpin %q %q\n", cid, err)
			C.execute_void_cb(fn, C.IPFS_PIN_FAILED, fn_arg)
			return
		}

		err = g.api.Pin().Add(g.ctx, path)

		if err != nil {
			fmt.Printf("go_asio_ipfs_pin failed to unpin %q %q\n", cid, err)
			C.execute_void_cb(fn, C.IPFS_PIN_FAILED, fn_arg)
			return
		}

		C.execute_void_cb(fn, C.IPFS_SUCCESS, fn_arg)
	}()
}

//export go_asio_ipfs_unpin
func go_asio_ipfs_unpin(c_cid *C.char, fn unsafe.Pointer, fn_arg unsafe.Pointer) {
	cid := C.GoString(c_cid)

	go func() {
		if debug {
			fmt.Println("go_asio_ipfs_unpin start");
			defer fmt.Println("go_asio_ipfs_unpin end");
		}

		path, err := coreapi.ParsePath(cid)

		if err != nil {
			fmt.Printf("go_asio_ipfs_pin failed to unpin %q %q\n", cid, err)
			C.execute_void_cb(fn, C.IPFS_PIN_FAILED, fn_arg)
			return
		}

		err = g.api.Pin().Rm(g.ctx, path)

		if err != nil {
			fmt.Printf("go_asio_ipfs_unpin failed to unpin %q %q\n", cid, err);
			C.execute_void_cb(fn, C.IPFS_UNPIN_FAILED, fn_arg)
			return
		}

		C.execute_void_cb(fn, C.IPFS_SUCCESS, fn_arg)
	}()
}

