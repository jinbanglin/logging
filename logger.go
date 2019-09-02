package log

import (
	"bufio"
	"github.com/jinbanglin/bytebufferpool"
	"github.com/jinbanglin/helper"
	"os"
	"sync"
)

type Hook interface {
	Fire(writer *bufio.Writer)
	Level(level)
}

type Logger struct {
	look            uint32
	link            string
	Path            string
	FileName        string
	file            *os.File
	fileWriter      *bufio.Writer
	timestamp       int
	FileMaxSize     int
	FileBufSize     int
	fileActualSize  int
	Bucket          chan *bytebufferpool.ByteBuffer
	lock            *sync.RWMutex
	closeSignal     chan string
	sigChan         chan os.Signal
	Persist         int
	sendEmail       *helper.Email
	RingInterval    int
	ContextTraceKey string
}
