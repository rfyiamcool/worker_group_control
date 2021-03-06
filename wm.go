package workerControl

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"time"
)

const (
	GracefulExitCode     = 0
	ForceTimeoutExitCode = 1

	logInfoLevel  = "info"
	logErrorLevel = "error"
)

var (
	WMCtl = NewWorkerControl()
)

type Worker interface {
	Start()
	Stop()
	GetProcessName() string
	//Status()
}

type workerControl struct {
	sync.WaitGroup
	WorkerSlice []Worker
	tryCatch    bool
	Running     bool
	Q           chan os.Signal
	Ctx         context.Context
	CtxCancel   context.CancelFunc
}

func NewWorkerControl() *workerControl {
	wm := workerControl{
		Running: true,
	}

	wm.WorkerSlice = make([]Worker, 0, 10)
	wm.Q = make(chan os.Signal)
	wm.Ctx, wm.CtxCancel = context.WithCancel(context.Background())

	return &wm
}

func (wm *workerControl) MakeSignal() {
	signal.Notify(wm.Q,
		syscall.SIGHUP,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGQUIT,
		os.Interrupt,
		os.Kill,
	)
}

func (wm *workerControl) RecvSignal() os.Signal {
	select {
	case s := <-wm.Q:
		defualtLogger(logInfoLevel, fmt.Sprintf("custom recv signale: %+v", s))
		return s
	}
}

func (wm *workerControl) MakeRecvSignal() os.Signal {
	wm.MakeSignal()
	return wm.RecvSignal()
}

func (wm *workerControl) AddWorker(w Worker) {
	wm.WorkerSlice = append(wm.WorkerSlice, w)
}

func (wm *workerControl) AddWorkerList(w []Worker) {
	wm.WorkerSlice = append(wm.WorkerSlice, w...)
}

func (wm *workerControl) SetTryCatch(b bool) {
	wm.tryCatch = true
}

func (wm *workerControl) Start() {
	for _, worker := range wm.WorkerSlice {
		go func(w Worker) {
			if wm.tryCatch {
				tryCatch(w)
				return
			}

			w.Start()
		}(worker)
	}
}

func (wm *workerControl) Stop() {
	wm.CtxCancel()     // stop
	wm.Running = false // stop
	for _, worker := range wm.WorkerSlice {
		go func(w Worker) {
			defer func() {
				err := recover()
				if err != nil {
					defualtLogger(logErrorLevel, fmt.Sprintf("workerControl error, error:%v, stack: %v\n",
						err, string(dumpStack())),
					)
				}
			}()

			w.Stop()
		}(worker)
	}
}

func (wm *workerControl) WaitTimeout(timeout int) int {
	endQ := make(chan bool, 0)
	go func() {
		defer close(endQ)
		wm.Wait()
	}()

	select {
	case <-endQ:
		return GracefulExitCode
	case <-time.After(time.Duration(timeout) * time.Second):
		return ForceTimeoutExitCode
	}
}

func tryCatch(f Worker) {
	running := true
	for running {
		func(w Worker) {
			defer func() {
				if e := recover(); e != nil {
					defualtLogger(logErrorLevel, fmt.Sprintf("worker_name: %s trycatch panicing %v ===> stask: %s \n",
						w.GetProcessName(),
						e,
						string(dumpStack())),
					)

					time.Sleep(3 * time.Second)
				}
			}()

			w.Start()

			// If worker is no except exit; don't need run again.
			running = false
			return
		}(f)
	}
}

// null logger
var defualtLogger = func(level, s string) {}

type loggerType func(level, s string)

func SetLogger(logger loggerType) {
	defualtLogger = logger
}

func dumpStack() []byte {
	buf := make([]byte, 12048)
	n := runtime.Stack(buf, false)
	return buf[:n]
}
