package log

type level = uint8
type coreStatus = uint32

const (
	_DEBUG level = iota + 1
	_INFO
	_WARN
	_ERR
	_DISASTER
)

const (
	B = 1 << (10 * iota)
	KB
	MB
	GB
	TB
	PB
)
const (
	OUT_STDOUT = 0x1f
	OUT_FILE   = 0x8b
)

var (
	coreDead    coreStatus = 2 //gLogger is dead
	coreRunning coreStatus = 1 //gLogger is running
)
