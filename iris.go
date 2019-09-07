package log

import (
	"github.com/google/uuid"
	"github.com/kataras/iris"
	"github.com/kataras/iris/context"
	"github.com/kataras/iris/middleware/logger"
	"strings"
	"time"
)

const TraceContextKey = "trace"
const LoggerMessageKey = "message"
const UserAgentKey = "User-Agent"
const ResponseDataKey = "data"

var excludeExtensions = [...]string{
	".js",
	".css",
	".jpg",
	".png",
	".ico",
	".svg",
}

func NewIrisLogger(config *logger.Config) iris.Handler {
	if config == nil {
		config = &logger.Config{
			//状态显示状态代码
			Status: true,
			// IP显示请求的远程地址
			IP: true,
			//方法显示http方法
			Method: true,
			// Path显示请求路径
			Path: true,
			// Query将url查询附加到Path。
			Query: true,
			// 如果不为空然后它的内容来自`ctx.Values(),Get("logger_message")
			MessageContextKeys: []string{LoggerMessageKey, TraceContextKey},
			//将添加到日志中。
			//如果不为空然后它的内容来自`ctx.GetHeader（“User-Agent”）
			MessageHeaderKeys: []string{UserAgentKey},
			LogFuncCtx: func(ctx iris.Context, latency time.Duration) {
				var file, line = ctx.HandlerFileLine()
				var messsage = ""
				if ctx.GetStatusCode() == iris.StatusOK {
					messsage = string(ctx.Values().Get(ResponseDataKey).([]byte))
					ctx.WriteString(messsage)
				} else {
					messsage = ctx.Values().GetString(LoggerMessageKey)
				}
				Infof3(" ❀  %s:%d |trace=%s |latency=%s |status=%d |method=%s |path=%s |message=%s |user-agent=%s |ip=%s",
					file, line,
					ctx.Values().GetString(TraceContextKey),
					latency.String(),
					ctx.GetStatusCode(),
					ctx.Method(),
					ctx.Path(),
					messsage,
					ctx.GetHeader(UserAgentKey),
					ctx.RemoteAddr(),
				)
			},
		}
		config.AddSkipper(func(ctx context.Context) bool {
			path := ctx.Path()
			for _, ext := range excludeExtensions {
				if strings.HasSuffix(path, ext) {
					return true
				}
			}
			return false
		})
	}
	return logger.New(*config)
}

func SetTraceMiddleware(ctx iris.Context) {
	ctx.Values().Set(TraceContextKey, uuid.New().String())
	ctx.Next()
}
