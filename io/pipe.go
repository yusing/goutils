package ioutils

import (
	"context"
	"errors"
	"io"
	"sync"
	"syscall"
)

type (
	Pipe struct {
		r ContextReader
		w ContextWriter
	}

	BidirectionalPipe struct {
		pSrcDst Pipe
		pDstSrc Pipe
	}
)

func NewPipe(ctx context.Context, r io.ReadCloser, w io.WriteCloser) Pipe {
	return Pipe{
		r: ContextReader{ctx: ctx, Reader: r},
		w: ContextWriter{ctx: ctx, Writer: w},
	}
}

func (p Pipe) Start() (err error) {
	err = CopyClose(&p.w, &p.r, 0)
	switch {
	case
		// NOTE: ignoring broken pipe and connection reset by peer
		errors.Is(err, syscall.EPIPE),
		errors.Is(err, syscall.ECONNRESET):
		return nil
	}
	return err
}

func NewBidirectionalPipe(ctx context.Context, rw1 io.ReadWriteCloser, rw2 io.ReadWriteCloser) BidirectionalPipe {
	return BidirectionalPipe{
		pSrcDst: NewPipe(ctx, rw1, rw2),
		pDstSrc: NewPipe(ctx, rw2, rw1),
	}
}

func (p BidirectionalPipe) Start() error {
	var wg sync.WaitGroup
	var srcErr, dstErr error
	wg.Go(func() {
		srcErr = p.pSrcDst.Start()
	})
	wg.Go(func() {
		dstErr = p.pDstSrc.Start()
	})
	wg.Wait()
	return errors.Join(srcErr, dstErr)
}
