(define-module (asio-ipfs)
  #:use-module (guix licenses)
  #:use-module (guix packages)
  #:use-module (guix download)
  #:use-module (guix git-download)
  #:use-module (guix gexp)
  #:use-module (guix build-system cmake)
  #:use-module (gnu packages)
  #:use-module (gnu packages base)
  #:use-module (gnu packages boost)
  #:use-module (gnu packages certs)
  #:use-module (gnu packages commencement)
  #:use-module (gnu packages compression)
  #:use-module (gnu packages golang)
  #:use-module (gnu packages tls)
  #:use-module (gnu packages patchutils)
  #:use-module (gnu packages rsync)
  #:use-module (gnu packages serialization)
  #:use-module (gnu packages version-control))

;; TODO: reuse (gnu packages ipfs)/go-ipfs src
(define %go-ipfs-src
  (origin
    (method url-fetch)
    (uri "https://dist.ipfs.io/go-ipfs/v0.4.19/go-ipfs-source.tar.gz")
    (sha256
     (base32
      "0s04ap14p6hnipjm27nm5k8s28zv9k5g9mziyh3ibgwn7dzb1kpx"))))
(define-public asio-ipfs
  (package
    (name "asio-ipfs")
    (version "0.0")
    (source
      (origin
        (method git-fetch)
        (uri (git-reference
          (url "https://github.com/equalitie/asio-ipfs.git")
          (commit "23641d2e6117fbe599ff151b16532b9a0ad5c36f")))
        (file-name (git-file-name name version))
        (sha256
          (base32 "0b3qczz55yk27xd51zhfg0fr8v90i40ga7ph5fzkpd9fmadya65s"))))
    (build-system cmake-build-system)
    (arguments
     '(#:tests? #f)) ; no tests
    (inputs
     `(("go" , go)
       ("go-ipfs-src" , %go-ipfs-src)
       ("nss-certs" , nss-certs)
       ("gcc-toolchain" , gcc-toolchain)
       ("rsync" , rsync)
       ("boost" , boost)))
    (synopsis "")
    (description "")
    (home-page "https://github.com/equalitie/asio-ipfs")
    (license expat)))
asio-ipfs
