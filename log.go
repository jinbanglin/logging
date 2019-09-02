package log

import (
	"bufio"
	"errors"
	"fmt"
	"github.com/jinbanglin/bytebufferpool"
	"github.com/jinbanglin/helper"
	"github.com/kataras/iris"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

func init() {
	if gLogger == nil {
		SetupLogger(nil)
	}
}

const tsLayout = "2006.01.02.15.04.05"

var gLogger *Logger

func SetupLogger(config *Logger) {
	if config == nil {
		fileName := "logs"
		gLogger = &Logger{
			look:            coreDead,
			FileName:        fileName,
			FileBufSize:     200 * MB,
			Path:            filepath.Join(getCurrentDirectory(), fileName),
			FileMaxSize:     1024 * MB,
			Bucket:          make(chan *bytebufferpool.ByteBuffer, 1024),
			closeSignal:     make(chan string),
			lock:            &sync.RWMutex{},
			sigChan:         make(chan os.Signal),
			Persist:         OUT_STDOUT,
			RingInterval:    500,
			ContextTraceKey: TraceContextKey,
		}
	} else {
		config.look = coreDead
		config.Path = filepath.Join(config.Path, config.FileName)
		config.closeSignal = make(chan string)
		if config.FileName == "" {
			config.FileName = "logs"
		}
		if config.FileMaxSize <= 0 {
			config.FileMaxSize = 1024 * MB
		}
		if len(config.Bucket) <= 0 {
			config.Bucket = make(chan *bytebufferpool.ByteBuffer, 1024)
		}
		if config.RingInterval <= 0 {
			config.RingInterval = 500
		}
		if config.Path == "" {
			config.Path = filepath.Join(getCurrentDirectory(), config.FileName)
		}
		if config.ContextTraceKey == "" {
			config.ContextTraceKey = TraceContextKey
		}
		config.lock = &sync.RWMutex{}
		gLogger = config
	}
	if gLogger.Persist == OUT_FILE {
		go poller()
	}
}

func (l *Logger) loadCurLogFile() error {
	l.link = filepath.Join(l.Path, gLogger.FileName+".log")
	actFileName, ok := isLinkFile(l.link)
	if !ok {
		return errors.New(l.link + " is not link file or not exist")
	}
	l.FileName = actFileName
	f, err := openFile(l.FileName)
	if err != nil {
		return err
	}
	info, err := os.Stat(l.FileName)
	if err != nil {
		return err
	}
	t, err := time.Parse(tsLayout, strings.TrimSuffix(path.Base(info.Name()), ".log"))
	if err != nil {
		fmt.Printf("Parse |err=%v \n", err)
		return err
	}
	y, m, d := t.Date()
	l.timestamp = y*10000 + int(m)*100 + d*1
	l.file = f
	l.fileActualSize = int(info.Size())
	l.fileWriter = bufio.NewWriterSize(f, l.FileBufSize)
	return nil
}

func (l *Logger) createFile() (err error) {
	if !pathIsExist(l.Path) {
		if err = os.MkdirAll(l.Path, os.ModePerm); err != nil {
			fmt.Printf("MkdirAll |err=%v \n ", err)
			return
		}
	}
	now := time.Now()
	y, m, d := now.Date()
	l.timestamp = y*10000 + int(m)*100 + d*1
	l.FileName = filepath.Join(
		l.Path,
		now.Format(tsLayout)+".log")
	f, err := openFile(l.FileName)
	if err != nil {
		fmt.Printf("openFile |err=%v \n", err)
		return err
	}
	l.file = f
	l.fileActualSize = 0
	l.fileWriter = bufio.NewWriterSize(f, l.FileBufSize)
	return os.Symlink(l.FileName, l.link)
}

func (l *Logger) sync() {
	if l.lookRunning() {
		err := l.fileWriter.Flush()
		if err != nil {
			fmt.Printf("sync |err=%v \n", err)
		}
	}
}

const fileMaxDelta = 100

func (l *Logger) rotate() bool {
	if !l.lookRunning() {
		return false
	}
	y, m, d := time.Now().Date()
	timestamp := y*10000 + int(m)*100 + d*1
	if l.fileActualSize <= l.FileMaxSize-fileMaxDelta && timestamp <= l.timestamp {
		return false
	}
	l.sync()
	closeFile(l.file)
	return l.createFile() == nil
}

func (l *Logger) lookRunning() bool { return atomic.LoadUint32(&l.look) == coreRunning }

func (l *Logger) lookDead() bool { return atomic.LoadUint32(&l.look) == coreDead }

func (l *Logger) signalHandler() {
	signal.Notify(
		l.sigChan,
		os.Interrupt,
		syscall.SIGINT, // register that too, it should be ok
		// os.Kill等同于syscall.Kill
		os.Kill,
		syscall.SIGKILL, // register that too, it should be ok
		// kill -SIGTERM XXXX
		syscall.SIGTERM)

	for {
		select {
		case sig := <-l.sigChan:
			l.closeSignal <- "close"
			fmt.Println("❀ log receive os signal is ", sig)
			l.sync()
			closeFile(l.file)
			atomic.SwapUint32(&l.look, coreDead)
			close(l.Bucket)
			fmt.Println("❀ log shutdown done success")
			os.Exit(1)
		}
	}
}

func (l *Logger) release(buf *bytebufferpool.ByteBuffer) { bytebufferpool.Put(buf) }

func caller() string {
	pc, f, l, _ := runtime.Caller(2)
	funcName := runtime.FuncForPC(pc).Name()
	return path.Base(f) + "/" + path.Base(funcName) + " [" + strconv.Itoa(l) + "] "
}

func Infof3(format string, msg ...interface{}) {
	buf := bytebufferpool.Get()
	buf.Write(string2Byte("[INFO] " + time.Now().Format("01/02/15:04:05")))
	buf.Write(string2Byte(fmt.Sprintf(format, msg...) + "\n"))
	flow(_INFO, buf)
}

func flow(lvl level, buf *bytebufferpool.ByteBuffer) {
	if gLogger.sendEmail != nil && lvl >= _ERR {
		helper.EmailInstance().SendMail(buf.String())
	}
	switch gLogger.Persist {
	case OUT_FILE:
		gLogger.Bucket <- buf
	case OUT_STDOUT:
		fmt.Print(buf.String())
	default:
		fmt.Print(buf.String())
	}
}

func Debugf(format string, msg ...interface{}) {
	buf := bytebufferpool.Get()
	buf.Write(string2Byte("[DEBU] " + time.Now().Format("01/02/15:04:05") + " " + caller() + "❀ "))
	buf.Write(string2Byte(fmt.Sprintf(format, msg...) + "\n"))
	flow(_DEBUG, buf)
}

func Infof(format string, msg ...interface{}) {
	buf := bytebufferpool.Get()
	buf.Write(string2Byte("[INFO] " + time.Now().Format("01/02/15:04:05") + " " + caller() + "❀ "))
	buf.Write(string2Byte(fmt.Sprintf(format, msg...) + "\n"))
	flow(_INFO, buf)
}

func Warnf(format string, msg ...interface{}) {
	buf := bytebufferpool.Get()
	buf.Write(string2Byte("[WARN] " + time.Now().Format("01/02/15:04:05") + " " + caller() + "❀ "))
	buf.Write(string2Byte(fmt.Sprintf(format, msg...) + "\n"))
	flow(_WARN, buf)
}

func Errorf(format string, msg ...interface{}) {
	buf := bytebufferpool.Get()
	buf.Write(string2Byte("[ERRO] " + time.Now().Format("01/02/15:04:05") + " " + caller() + "❀ "))
	buf.Write(string2Byte(fmt.Sprintf(format, msg...) + "\n"))
	flow(_ERR, buf)
}

func Fatalf(format string, msg ...interface{}) {
	buf := bytebufferpool.Get()
	buf.Write(string2Byte("[FTAL] " + time.Now().Format("01/02/15:04:05") + " " + caller() + "❀ "))
	buf.Write(string2Byte(fmt.Sprintf(format, msg...) + "\n"))
	flow(_DISASTER, buf)
}

func Stackf(format string, msg ...interface{}) {
	s := fmt.Sprintf(format, msg...)
	s += "\n"
	buf := make([]byte, 1<<20)
	n := runtime.Stack(buf, true)
	s += string(buf[:n])
	s += "\n"
	fmt.Println("[STAC][" + time.Now().Format("01/02/15:04:05") + "]" + "[" + caller() + "] ❀ " + s)
}

func Debug(msg ...interface{}) {
	buf := bytebufferpool.Get()
	buf.Write(string2Byte("[DEBU] " + time.Now().Format("01/02/15:04:05") + " " + caller() + "❀ "))
	buf.Write(string2Byte(fmt.Sprintln(msg...)))
	flow(_DEBUG, buf)
}

func Info(msg ...interface{}) {
	buf := bytebufferpool.Get()
	buf.Write(string2Byte("[INFO] " + time.Now().Format("01/02/15:04:05") + " " + caller() + "❀ "))
	buf.Write(string2Byte(fmt.Sprintln(msg...)))
	flow(_INFO, buf)
}

func Warn(msg ...interface{}) {
	buf := bytebufferpool.Get()
	buf.Write(string2Byte("[WARN] " + time.Now().Format("01/02/15:04:05") + " " + caller() + "❀ "))
	buf.Write(string2Byte(fmt.Sprintln(msg...)))
	flow(_WARN, buf)
}

func Error(msg ...interface{}) {
	buf := bytebufferpool.Get()
	buf.Write(string2Byte("[ERRO] " + time.Now().Format("01/02/15:04:05") + " " + caller() + "❀ "))
	buf.Write(string2Byte(fmt.Sprintln(msg...)))
	flow(_ERR, buf)
}

func Fatal(msg ...interface{}) {
	buf := bytebufferpool.Get()
	buf.Write(string2Byte("[FTAL] " + time.Now().Format("01/02/15:04:05") + " " + caller() + "❀ "))
	buf.Write(string2Byte(fmt.Sprintln(msg...)))
	flow(_DISASTER, buf)
}

func Stack(msg ...interface{}) {
	s := fmt.Sprintln(msg...)
	s += "\n"
	buf := make([]byte, 1<<20)
	n := runtime.Stack(buf, true)
	s += string(buf[:n])
	s += "\n"
	fmt.Println("[STAC][" + time.Now().Format("01/02/15:04:05") + "]" + "[" + caller() + "] ❀ " + s)
}

func Debugf2(ctx iris.Context, format string, msg ...interface{}) {
	buf := bytebufferpool.Get()
	buf.Write(string2Byte("[DEBU] " + time.Now().Format("01/02/15:04:05") + " " + caller() + "❀ "))
	buf.Write(string2Byte(gLogger.ContextTraceKey + "=" + ctx.Values().GetString(TraceContextKey)+ " |"))
	buf.Write(string2Byte(fmt.Sprintf(format, msg...) + "\n"))
	flow(_DEBUG, buf)
}

func Infof2(ctx iris.Context, format string, msg ...interface{}) {
	buf := bytebufferpool.Get()
	buf.Write(string2Byte("[INFO] " + time.Now().Format("01/02/15:04:05") + " " + caller() + "❀ "))
	buf.Write(string2Byte(gLogger.ContextTraceKey + "=" + ctx.Values().GetString(TraceContextKey)+ " |"))
	buf.Write(string2Byte(fmt.Sprintf(format, msg...) + "\n"))
	flow(_INFO, buf)
}

func Warnf2(ctx iris.Context, format string, msg ...interface{}) {
	buf := bytebufferpool.Get()
	buf.Write(string2Byte("[WARN] " + time.Now().Format("01/02/15:04:05") + " " + caller() + "❀ "))
	buf.Write(string2Byte(gLogger.ContextTraceKey + "=" + ctx.Values().GetString(TraceContextKey)+ " |"))
	buf.Write(string2Byte(fmt.Sprintf(format, msg...) + "\n"))
	flow(_WARN, buf)
}

func Errorf2(ctx iris.Context, format string, msg ...interface{}) {
	buf := bytebufferpool.Get()
	buf.Write(string2Byte("[ERRO] " + time.Now().Format("01/02/15:04:05") + " " + caller() + "❀ "))
	buf.Write(string2Byte(gLogger.ContextTraceKey + "=" + ctx.Values().GetString(TraceContextKey)+ " |"))
	buf.Write(string2Byte(fmt.Sprintf(format, msg...) + "\n"))
	flow(_ERR, buf)
}

func Fatalf2(ctx iris.Context, format string, msg ...interface{}) {
	buf := bytebufferpool.Get()
	buf.Write(string2Byte("[FTAL] " + time.Now().Format("01/02/15:04:05") + " " + caller() + "❀ "))
	buf.Write(string2Byte(gLogger.ContextTraceKey + "=" + ctx.Values().GetString(TraceContextKey)+ " |"))
	buf.Write(string2Byte(fmt.Sprintf(format, msg...) + "\n"))
	flow(_DISASTER, buf)
}

func Debug2(ctx iris.Context, msg ...interface{}) {
	buf := bytebufferpool.Get()
	buf.Write(string2Byte("[DEBU] " + time.Now().Format("01/02/15:04:05") + " " + caller() + "❀ "))
	buf.Write(string2Byte(gLogger.ContextTraceKey + "=" + ctx.Values().GetString(TraceContextKey)+ " |"))
	buf.Write(string2Byte(fmt.Sprintln(msg...)))
	flow(_DEBUG, buf)
}

func Info2(ctx iris.Context, msg ...interface{}) {
	buf := bytebufferpool.Get()
	buf.Write(string2Byte("[INFO] " + time.Now().Format("01/02/15:04:05") + " " + caller() + "❀ "))
	buf.Write(string2Byte(gLogger.ContextTraceKey + "=" + ctx.Values().GetString(TraceContextKey)+ " |"))
	buf.Write(string2Byte(fmt.Sprintln(msg...)))
	flow(_INFO, buf)
}

func Warn2(ctx iris.Context, msg ...interface{}) {
	buf := bytebufferpool.Get()
	buf.Write(string2Byte("[WARN] " + time.Now().Format("01/02/15:04:05") + " " + caller() + "❀ "))
	buf.Write(string2Byte(gLogger.ContextTraceKey + "=" + ctx.Values().GetString(TraceContextKey)+ " |"))
	buf.Write(string2Byte(fmt.Sprintln(msg...)))
	flow(_WARN, buf)
}

func Error2(ctx iris.Context, msg ...interface{}) {
	buf := bytebufferpool.Get()
	buf.Write(string2Byte("[ERRO] " + time.Now().Format("01/02/15:04:05") + " " + caller() + "❀ "))
	buf.Write(string2Byte(gLogger.ContextTraceKey + "=" + ctx.Values().GetString(TraceContextKey)+ " |"))
	buf.Write(string2Byte(fmt.Sprintln(msg...)))
	flow(_ERR, buf)
}

func Fatal2(ctx iris.Context, msg ...interface{}) {
	buf := bytebufferpool.Get()
	buf.Write(string2Byte("[FTAL] " + time.Now().Format("01/02/15:04:05") + " " + caller() + "❀ "))
	buf.Write(string2Byte(gLogger.ContextTraceKey + "=" + ctx.Values().GetString(TraceContextKey)+ " |"))
	buf.Write(string2Byte(fmt.Sprintln(msg...)))
	flow(_DISASTER, buf)
}

func string2Byte(s string) []byte {
	return *(*[]byte)(unsafe.Pointer(&s))
}

