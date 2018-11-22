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

	peer "gx/ipfs/QmdVrMn1LhB4ybb8hMVaMLXnA8XRSewMnK6YqXKXoTcRvN/go-libp2p-peer"
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

type Node struct {
	node *core.IpfsNode
	api coreiface.CoreAPI
	ctx context.Context
	cancel context.CancelFunc

	next_cancel_signal_id C.uint64_t
	cancel_signals map[C.uint64_t]func()
}

// Both object tables, and assorted ID trackers,
// are only ever accessed from the C thread.

var g_next_node_id uint64 = 0
var g_nodes = make(map[uint64]*Node)


//export go_asio_ipfs_allocate
func go_asio_ipfs_allocate() uint64 {
	var n Node

	n.ctx, n.cancel = context.WithCancel(context.Background())

	n.next_cancel_signal_id = 0
	n.cancel_signals = make(map[C.uint64_t]func())

	ret := g_next_node_id
	g_nodes[g_next_node_id] = &n
	g_next_node_id += 1
	
	return ret
}

//export go_asio_ipfs_free
func go_asio_ipfs_free(handle uint64) {
	g_nodes[handle].cancel()
	delete(g_nodes, handle)
}


//export go_asio_ipfs_cancellation_allocate
func go_asio_ipfs_cancellation_allocate(handle uint64) C.uint64_t {
	n, ok := g_nodes[handle]
	if !ok { return C.uint64_t(1<<64 - 1 /* max uint64 */) }

	ret := n.next_cancel_signal_id
	n.next_cancel_signal_id += 1
	return ret
}

//export go_asio_ipfs_cancellation_free
func go_asio_ipfs_cancellation_free(handle uint64, cancel_signal C.uint64_t) {
	n, ok := g_nodes[handle]
	if !ok { return }

	delete(n.cancel_signals, cancel_signal)
}

func withCancel(n *Node, cancel_signal C.uint64_t) (context.Context) {
	ctx, cancel := context.WithCancel(n.ctx)
	n.cancel_signals[cancel_signal] = cancel
	return ctx
}

//export go_asio_ipfs_cancel
func go_asio_ipfs_cancel(handle uint64, cancel_signal C.uint64_t) {
	n, ok := g_nodes[handle]
	if !ok { return }

	cancel, ok := n.cancel_signals[cancel_signal]
	if !ok { return }

	cancel()
}


func start_node(n *Node, repoRoot string) C.int {
	r, err := openOrCreateRepo(repoRoot);

	if err != nil {
		fmt.Println("err", err);
		return C.IPFS_FAILED_TO_CREATE_REPO
	}

	n.node, err = core.NewNode(n.ctx, &core.BuildCfg{
		Online: true,
		Permanent: true,
		Repo:   r,
		ExtraOpts: map[string]bool{
			"ipnsps": enablePubSubIPNS,
		},
	})

	n.node.SetLocal(false)

	printSwarmAddrs(n.node)

	n.api = coreapi.NewCoreAPI(n.node)

	return C.IPFS_SUCCESS
}

//export go_asio_ipfs_start_blocking
func go_asio_ipfs_start_blocking(handle uint64, c_repoPath *C.char) C.int {
	var n = g_nodes[handle]

	repoRoot := C.GoString(c_repoPath)

	return start_node(n, repoRoot);
}

//export go_asio_ipfs_start_async
func go_asio_ipfs_start_async(handle uint64, c_repoPath *C.char, fn unsafe.Pointer, fn_arg unsafe.Pointer) {
	var n = g_nodes[handle]

	repoRoot := C.GoString(c_repoPath)

	go func() {
		err := start_node(n, repoRoot);

		C.execute_void_cb(fn, err, fn_arg)
	}()
}


// IMPORTANT: The returned value needs to be explicitly `free`d.
//export go_asio_ipfs_node_id
func go_asio_ipfs_node_id(handle uint64) *C.char {
	var n = g_nodes[handle]

	pid, err := peer.IDFromPrivateKey(n.node.PrivateKey)

	if err != nil {
		return nil
	}

	cstr := C.CString(pid.Pretty())
	return cstr
}

//export go_asio_ipfs_resolve
func go_asio_ipfs_resolve(handle uint64, cancel_signal C.uint64_t, c_ipns_id *C.char, fn unsafe.Pointer, fn_arg unsafe.Pointer) {
	var n = g_nodes[handle]

	ipns_id := C.GoString(c_ipns_id)

	cancel_ctx := withCancel(n, cancel_signal)

	go func() {
		if debug {
			fmt.Println("go_asio_ipfs_resolve start");
			defer fmt.Println("go_asio_ipfs_resolve end");
		}

		n := n.node
		p := path.Path("/ipns/" + ipns_id)

		node, err := core.Resolve(cancel_ctx, n.Namesys, n.Resolver, p)

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
func go_asio_ipfs_publish(handle uint64, cancel_signal C.uint64_t, cid *C.char, seconds C.int64_t, fn unsafe.Pointer, fn_arg unsafe.Pointer) {
	var n = g_nodes[handle]

	id := C.GoString(cid)

	cancel_ctx := withCancel(n, cancel_signal)

	go func() {
		if debug {
			fmt.Println("go_asio_ipfs_publish start");
			defer fmt.Println("go_asio_ipfs_publish end");
		}

		// https://stackoverflow.com/questions/17573190/how-to-multiply-duration-by-integer
		err := publish(cancel_ctx, time.Duration(seconds) * time.Second, n.node, id);

		if err != nil {
			C.execute_void_cb(fn, C.IPFS_PUBLISH_FAILED, fn_arg)
			return
		}

		C.execute_void_cb(fn, C.IPFS_SUCCESS, fn_arg)
	}()
}

//export go_asio_ipfs_add
func go_asio_ipfs_add(handle uint64, data unsafe.Pointer, size C.size_t, fn unsafe.Pointer, fn_arg unsafe.Pointer) {
	var n = g_nodes[handle]

	msg := C.GoBytes(data, C.int(size))

	go func() {
		if debug {
			fmt.Println("go_asio_ipfs_add start");
			defer fmt.Println("go_asio_ipfs_add end");
		}

		cid, err := coreunix.Add(n.node, bytes.NewReader(msg))

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
func go_asio_ipfs_cat(handle uint64, cancel_signal C.uint64_t, c_cid *C.char, fn unsafe.Pointer, fn_arg unsafe.Pointer) {
	var n = g_nodes[handle]

	cid := C.GoString(c_cid)

	cancel_ctx := withCancel(n, cancel_signal)

	go func() {
		if debug {
			fmt.Println("go_asio_ipfs_cat start");
			defer fmt.Println("go_asio_ipfs_cat end");
		}

		reader, err := coreunix.Cat(cancel_ctx, n.node, cid)

		if err != nil {
			fmt.Printf("go_asio_ipfs_cat failed to Cat %q\n", err);
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
func go_asio_ipfs_pin(handle uint64, cancel_signal C.uint64_t, c_cid *C.char, fn unsafe.Pointer, fn_arg unsafe.Pointer) {
	var n = g_nodes[handle]

	cid := C.GoString(c_cid)

	cancel_ctx := withCancel(n, cancel_signal)

	go func() {
		if debug {
			fmt.Println("go_asio_ipfs_pin start");
			defer fmt.Println("go_asio_ipfs_pin end");
		}

		path, err := coreiface.ParsePath(cid)

		if err != nil {
			fmt.Printf("go_asio_ipfs_pin failed to unpin %q %q\n", cid, err)
			C.execute_void_cb(fn, C.IPFS_PIN_FAILED, fn_arg)
			return
		}

		err = n.api.Pin().Add(cancel_ctx, path)

		if err != nil {
			fmt.Printf("go_asio_ipfs_pin failed to unpin %q %q\n", cid, err)
			C.execute_void_cb(fn, C.IPFS_PIN_FAILED, fn_arg)
			return
		}

		C.execute_void_cb(fn, C.IPFS_SUCCESS, fn_arg)
	}()
}

//export go_asio_ipfs_unpin
func go_asio_ipfs_unpin(handle uint64, cancel_signal C.uint64_t, c_cid *C.char, fn unsafe.Pointer, fn_arg unsafe.Pointer) {
	var n = g_nodes[handle]

	cid := C.GoString(c_cid)

	cancel_ctx := withCancel(n, cancel_signal)

	go func() {
		if debug {
			fmt.Println("go_asio_ipfs_unpin start");
			defer fmt.Println("go_asio_ipfs_unpin end");
		}

		path, err := coreiface.ParsePath(cid)

		if err != nil {
			fmt.Printf("go_asio_ipfs_pin failed to unpin %q %q\n", cid, err)
			C.execute_void_cb(fn, C.IPFS_PIN_FAILED, fn_arg)
			return
		}

		err = n.api.Pin().Rm(cancel_ctx, path)

		if err != nil {
			fmt.Printf("go_asio_ipfs_unpin failed to unpin %q %q\n", cid, err);
			C.execute_void_cb(fn, C.IPFS_UNPIN_FAILED, fn_arg)
			return
		}

		C.execute_void_cb(fn, C.IPFS_SUCCESS, fn_arg)
	}()
}

