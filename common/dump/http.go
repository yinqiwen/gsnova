package dump

import (
	"bufio"
	"bytes"
	"compress/flate"
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httputil"
	"os"
	"strings"
	"sync"

	"github.com/dsnet/compress/brotli"
	"github.com/yinqiwen/gotoolkit/iotools"
	"github.com/yinqiwen/gsnova/common/helper"
)

var dumpFiles = make(map[string]*iotools.RotateFile)
var dumpFilesMutex sync.Mutex

func getDumpFile(file string) *iotools.RotateFile {
	dumpFilesMutex.Lock()
	f := dumpFiles[file]
	if nil == f {
		f = &iotools.RotateFile{
			Path:            file,
			MaxBackupIndex:  2,
			MaxFileSize:     1024 * 1024,
			SyncBytesPeriod: 1024 * 1024,
		}
		dumpFiles[file] = f
	}
	dumpFilesMutex.Unlock()
	return f
}

type HttpDumpReadWriter struct {
	R io.Reader
	W io.Writer

	notHTTP        bool
	isTLS          bool
	requestWriter  *io.PipeWriter
	responseWriter *io.PipeWriter
	reqReader      *io.PipeReader
	resReader      *io.PipeReader
	requestReader  *bufio.Reader
	responseReader *bufio.Reader

	dumpWriter io.Writer

	excludeBody []string
	includeBody []string
}

func decompressReader(r io.ReadCloser, header http.Header) io.ReadCloser {
	switch header.Get("Content-Encoding") {
	case "gzip":
		reader, err := gzip.NewReader(r)
		if nil == err {
			return ioutil.NopCloser(reader)
		}
		return r
	case "deflate":
		return flate.NewReader(r)
	case "br":
		reader, _ := brotli.NewReader(r, nil)
		return ioutil.NopCloser(reader)
	default:
		return r
	}
}

func (h *HttpDumpReadWriter) shouldDumpBody(header http.Header) bool {
	t := header.Get("Content-Type")
	if strings.Contains(t, "image") || strings.Contains(t, "binary") || strings.Contains(t, "video") || strings.Contains(t, "octet-stream") {
		return false
	}
	for _, exclude := range h.excludeBody {
		if strings.Contains(t, exclude) {
			return false
		}
	}
	if len(h.includeBody) > 0 {
		for _, include := range h.includeBody {
			if strings.Contains(t, include) {
				return true
			}
		}
		return false
	}
	return true
}
func (h *HttpDumpReadWriter) closeReader() error {
	if nil != h.reqReader {
		h.reqReader.Close()
	}
	if nil != h.resReader {
		h.resReader.Close()
	}
	return nil
}
func (h *HttpDumpReadWriter) closeWriter() error {
	if nil != h.requestWriter {
		h.requestWriter.Close()
	}
	if nil != h.responseWriter {
		h.responseWriter.Close()
	}

	return nil
}
func (h *HttpDumpReadWriter) Close() error {
	h.closeReader()
	h.closeWriter()
	return nil
}

func (h *HttpDumpReadWriter) Read(p []byte) (n int, err error) {
	n, err = h.R.Read(p)
	if n > 0 && !h.notHTTP {
		h.responseWriter.Write(p[0:n])
	}
	return
}
func (h *HttpDumpReadWriter) Write(p []byte) (n int, err error) {
	n, err = h.W.Write(p)
	if n > 0 && !h.notHTTP {
		h.requestWriter.Write(p[0:n])
	}
	return
}

func (h *HttpDumpReadWriter) dumpRecord(req *http.Request, res *http.Response) {
	var buf bytes.Buffer
	if nil != req {
		if !strings.Contains(req.RequestURI, "://") {
			if h.isTLS {
				req.RequestURI = "https://" + req.Host + req.RequestURI
			} else {
				req.RequestURI = "http://" + req.Host + req.RequestURI
			}
		}

		req.Body = decompressReader(req.Body, req.Header)
		content, _ := httputil.DumpRequest(req, h.shouldDumpBody(req.Header))
		buf.Write(content)
	}
	if nil != res {
		fmt.Fprintf(&buf, "->\n")
		res.Body = decompressReader(res.Body, res.Header)
		content, _ := httputil.DumpResponse(res, h.shouldDumpBody(res.Header))
		buf.Write(content)
	}
	fmt.Fprintf(&buf, "\n")
	io.Copy(h.dumpWriter, &buf)
}

func (h *HttpDumpReadWriter) dump() {

	for {
		req, err := http.ReadRequest(h.requestReader)
		if nil != err {
			h.closeReader()
			return
		}
		res, err := http.ReadResponse(h.responseReader, req)
		if nil != err {
			h.dumpRecord(req, nil)
			h.closeReader()
			return
		}
		h.dumpRecord(req, res)
		//content, _ = httputil.DumpResponse(res, true)
	}
}

type HttpDumpOptions struct {
	IncludeBody []string
	ExcludeBody []string
	Destination string
	IsTLS       bool
}

func NewHttpDumpReadWriter(r io.Reader, w io.Writer, options *HttpDumpOptions) *HttpDumpReadWriter {
	h := &HttpDumpReadWriter{
		R:           r,
		W:           w,
		isTLS:       options.IsTLS,
		includeBody: options.IncludeBody,
		excludeBody: options.ExcludeBody,
	}
	pr, pw := io.Pipe()
	h.requestWriter = pw
	h.requestReader = bufio.NewReader(pr)
	h.reqReader = pr
	pr, pw = io.Pipe()
	h.responseWriter = pw
	h.responseReader = bufio.NewReader(pr)
	h.resReader = pr

	if len(options.Destination) == 0 {
		h.dumpWriter = os.Stdout
	} else {
		if strings.HasPrefix(options.Destination, "http://") || strings.HasPrefix(options.Destination, "https://") {
			h.dumpWriter = &helper.HttpPostWriter{URL: options.Destination}
		} else {
			h.dumpWriter = getDumpFile(options.Destination)
		}
	}

	go h.dump()
	return h
}
