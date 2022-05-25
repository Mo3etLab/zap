// Copyright (c) 2016 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package zap

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"go.uber.org/zap/internal/bufferpool"
	"go.uber.org/zap/zapcore"
)

// A Logger provides fast, leveled, structured logging. All methods are safe
// for concurrent use.

// The Logger is designed for contexts in which every microsecond and every
// allocation matters, so its API intentionally favors performance and type
// safety over brevity. For most applications, the SugaredLogger strikes a
// better balance between performance and ergonomics.

// Logger 提供快速、分级、结构化的日志记录。 所有方法都是安全的
// 并发使用。

// Logger 是为每微秒和每
// 分配很重要，所以它的 API 有意偏向于性能和类型
// 安全胜过简洁。 对于大多数应用程序，SugaredLogger 打击了
// 在性能和人体工程学之间取得更好的平衡。

type Logger struct {
	core zapcore.Core

	development bool
	addCaller   bool
	onFatal     zapcore.CheckWriteHook // default is WriteThenFatal

	name        string
	errorOutput zapcore.WriteSyncer

	addStack zapcore.LevelEnabler

	callerSkip int

	clock zapcore.Clock
}

// New constructs a new Logger from the provided zapcore.Core and Options. If
// the passed zapcore.Core is nil, it falls back to using a no-op
// implementation.
//
// This is the most flexible way to construct a Logger, but also the most
// verbose. For typical use cases, the highly-opinionated presets
// (NewProduction, NewDevelopment, and NewExample) or the Config struct are
// more convenient.
//
// For sample code, see the package-level AdvancedConfiguration example.

// New 从提供的 zapcore.Core 和 Options 构造一个新的 Logger。 如果
// 传递的 zapcore.Core 为 nil，它回退到使用无操作
// 执行。
//
// 这是构造Logger最灵活的方式，也是最
// 详细。 对于典型的用例，高度评价的预设
// (NewProduction、NewDevelopment 和 NewExample) 或 Config 结构是
// 更方便。
//
// 示例代码请参见包级 AdvancedConfiguration 示例。

func New(core zapcore.Core, options ...Option) *Logger {
	if core == nil {
		return NewNop()
	}
	log := &Logger{
		core:        core,
		errorOutput: zapcore.Lock(os.Stderr),
		addStack:    zapcore.FatalLevel + 1,
		clock:       zapcore.DefaultClock,
	}
	return log.WithOptions(options...)
}

// NewNop returns a no-op Logger. It never writes out logs or internal errors,
// and it never runs user-defined hooks.
//
// Using WithOptions to replace the Core or error output of a no-op Logger can
// re-enable logging.

// NewNop 返回一个无操作记录器。 它从不写出日志或内部错误，
// 而且它从不运行用户定义的钩子。
//
// 使用WithOptions替换一个no-op Logger的Core或者错误输出可以
// 重新启用日志记录。

func NewNop() *Logger {
	return &Logger{
		core:        zapcore.NewNopCore(),
		errorOutput: zapcore.AddSync(ioutil.Discard),
		addStack:    zapcore.FatalLevel + 1,
		clock:       zapcore.DefaultClock,
	}
}

// NewProduction builds a sensible production Logger that writes InfoLevel and
// New Production构建了一个明智的生产记录仪
// above logs to standard error as JSON.
//上面的日志为标准错误作为JSON。
//
// It's a shortcut for NewProductionConfig().Build(...Option).
//这是NewProductionConfig（）。build（... option）的快捷方式。

func NewProduction(options ...Option) (*Logger, error) {
	return NewProductionConfig().Build(options...)
}

// NewDevelopment builds a development Logger that writes DebugLevel and above
// NewDevelopment建立了一个开发记录仪，该记录仪撰写Debuglevel及以上
// logs to standard error in a human-friendly format.
//以人为友好的格式记录到标准错误。
//
// It's a shortcut for NewDevelopmentConfig().Build(...Option).
//这是NewDevelopmentConfig（）。build（... option）的快捷方式。
func NewDevelopment(options ...Option) (*Logger, error) {
	return NewDevelopmentConfig().Build(options...)
}

// NewExample builds a Logger that's designed for use in zap's testable
// examples. It writes DebugLevel and above logs to standard out as JSON, but
// omits the timestamp and calling function to keep example output
// short and deterministic.

// NewExample 构建一个专门用于 zap 的可测试的 Logger
// 例子。 它将 DebugLevel 及以上的日志作为 JSON 写入标准输出，但是
// 省略时间戳和调用函数以保留示例输出
// 简短而确定。

func NewExample(options ...Option) *Logger {
	encoderCfg := zapcore.EncoderConfig{
		MessageKey:     "msg",
		LevelKey:       "level",
		NameKey:        "logger",
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
	}
	core := zapcore.NewCore(zapcore.NewJSONEncoder(encoderCfg), os.Stdout, DebugLevel)
	return New(core).WithOptions(options...)
}

// Sugar wraps the Logger to provide a more ergonomic, but slightly slower,
// API. Sugaring a Logger is quite inexpensive, so it's reasonable for a
// single application to use both Loggers and SugaredLoggers, converting
// between them on the boundaries of performance-sensitive code.

// Sugar 包装了 Logger 以提供更符合人体工程学，但速度稍慢，
// API。 给 Logger 加糖相当便宜，所以对于一个 Logger 来说是合理的
// 单个应用程序同时使用 Loggers 和 SugaredLoggers，转换
// 它们之间在性能敏感代码的边界上。

func (log *Logger) Sugar() *SugaredLogger {
	core := log.clone()
	core.callerSkip += 2
	return &SugaredLogger{core}
}

// Named adds a new path segment to the logger's name. Segments are joined by
// periods. By default, Loggers are unnamed.

// Named 为记录器的名称添加一个新的路径段。 细分由
// 句点。 默认情况下，Logger 是未命名的。

func (log *Logger) Named(s string) *Logger {
	if s == "" {
		return log
	}
	l := log.clone()
	if log.name == "" {
		l.name = s
	} else {
		l.name = strings.Join([]string{l.name, s}, ".")
	}
	return l
}

// WithOptions clones the current Logger, applies the supplied Options, and
//使用“使用克隆”当前记录器，应用所提供的选项，并且
// returns the resulting Logger. It's safe to use concurrently.
//返回结果记录器。同时使用安全。
func (log *Logger) WithOptions(opts ...Option) *Logger {
	c := log.clone()
	for _, opt := range opts {
		opt.apply(c)
	}
	return c
}

// With creates a child logger and adds structured context to it. Fields added
//使用创建子记录仪并在其中添加结构化上下文。添加了字段
// to the child don't affect the parent, and vice versa.
//对孩子不影响父母，反之亦然。
func (log *Logger) With(fields ...Field) *Logger {
	if len(fields) == 0 {
		return log
	}
	l := log.clone()
	l.core = l.core.With(fields)
	return l
}

// Check returns a CheckedEntry if logging a message at the specified level
//检查如果在指定级别记录消息，请返回checkedentry
// is enabled. It's a completely optional optimization; in high-performance
//已启用。这是一个完全可选的优化；高性能
// applications, Check can help avoid allocating a slice to hold fields.
//应用程序，检查可以帮助避免分配切片以容纳字段。
func (log *Logger) Check(lvl zapcore.Level, msg string) *zapcore.CheckedEntry {
	return log.check(lvl, msg)
}

// Debug logs a message at DebugLevel. The message includes any fields passed
// Debug在Debuglevel登录一条消息。该消息包括通过的任何字段
// at the log site, as well as any fields accumulated on the logger.
//在日志站点以及日志仪上积累的任何字段。
func (log *Logger) Debug(msg string, fields ...Field) {
	if ce := log.check(DebugLevel, msg); ce != nil {
		ce.Write(fields...)
	}
}

// Info logs a message at InfoLevel. The message includes any fields passed
//在日志站点以及日志仪上积累的任何字段。
// at the log site, as well as any fields accumulated on the logger.
//在日志站点以及日志仪上积累的任何字段。
func (log *Logger) Info(msg string, fields ...Field) {
	if ce := log.check(InfoLevel, msg); ce != nil {
		ce.Write(fields...)
	}
}

// Warn logs a message at WarnLevel. The message includes any fields passed
//警告在Warnlevel上记录一条消息。该消息包括通过的任何字段
// at the log site, as well as any fields accumulated on the logger.
//在日志站点以及日志仪上积累的任何字段。
func (log *Logger) Warn(msg string, fields ...Field) {
	if ce := log.check(WarnLevel, msg); ce != nil {
		ce.Write(fields...)
	}
}

// Error logs a message at ErrorLevel. The message includes any fields passed
//错误在ErrorLevel上记录消息。该消息包括通过的任何字段
// at the log site, as well as any fields accumulated on the logger.
//在日志站点以及日志仪上积累的任何字段。
func (log *Logger) Error(msg string, fields ...Field) {
	if ce := log.check(ErrorLevel, msg); ce != nil {
		ce.Write(fields...)
	}
}

// DPanic logs a message at DPanicLevel. The message includes any fields
// passed at the log site, as well as any fields accumulated on the logger.
//
// If the logger is in development mode, it then panics (DPanic means
// "development panic"). This is useful for catching errors that are
// recoverable, but shouldn't ever happen.

// DPanic 在 DPanicLevel 记录一条消息。 消息包括任何字段
// 在日志站点传递，以及在记录器上累积的任何字段。
//
// 如果记录器处于开发模式，则它会发生恐慌（DPanic 表示
// “发展恐慌”）。 这对于捕获错误很有用
// 可恢复，但不应该发生。

func (log *Logger) DPanic(msg string, fields ...Field) {
	if ce := log.check(DPanicLevel, msg); ce != nil {
		ce.Write(fields...)
	}
}

// Panic logs a message at PanicLevel. The message includes any fields passed
// at the log site, as well as any fields accumulated on the logger.
//
// The logger then panics, even if logging at PanicLevel is disabled.

// Panic 在 PanicLevel 记录一条消息。 该消息包括传递的任何字段
// 在日志站点，以及记录器上累积的任何字段。
//
// 然后记录器会恐慌，即使 PanicLevel 的记录被禁用。

func (log *Logger) Panic(msg string, fields ...Field) {
	if ce := log.check(PanicLevel, msg); ce != nil {
		ce.Write(fields...)
	}
}

// Fatal logs a message at FatalLevel. The message includes any fields passed
// at the log site, as well as any fields accumulated on the logger.
//
// The logger then calls os.Exit(1), even if logging at FatalLevel is
// disabled.

// Fatal 在 FatalLevel 记录一条消息。 该消息包括传递的任何字段
// 在日志站点，以及记录器上累积的任何字段。
//
// 然后记录器调用 os.Exit(1)，即使在 FatalLevel 的记录是
// 禁用。

func (log *Logger) Fatal(msg string, fields ...Field) {
	if ce := log.check(FatalLevel, msg); ce != nil {
		ce.Write(fields...)
	}
}

// Sync calls the underlying Core's Sync method, flushing any buffered log
// entries. Applications should take care to call Sync before exiting.

// Sync 调用底层 Core 的 Sync 方法，刷新所有缓冲的日志
// 条目。 应用程序应注意在退出之前调用 Sync。
func (log *Logger) Sync() error {
	return log.core.Sync()
}

// Core returns the Logger's underlying zapcore.Core.

//核心返回记录器的基础zapcore.core。
func (log *Logger) Core() zapcore.Core {
	return log.core
}

func (log *Logger) clone() *Logger {
	copy := *log
	return &copy
}

func (log *Logger) check(lvl zapcore.Level, msg string) *zapcore.CheckedEntry {
	// Logger.check must always be called directly by a method in the
	// logger.check必须始终通过一种方法在
	// Logger interface (e.g., Check, Info, Fatal).
	// Logger接口（例如，检查，信息，致命）。
	// This skips Logger.check and the Info/Fatal/Check/etc. method that
	//此跳过logger.check和info/fatal/check/等。方法
	// called it.
	// 叫它。

	// Logger.check 必须始终由
	// 记录器接口（例如，Check、Info、Fatal）。
	// 这会跳过 Logger.check 和 Info/Fatal/Check/etc。 方法
	// 叫它。

	const callerSkipOffset = 2

	// Check the level first to reduce the cost of disabled log calls.
	//首先检查级别以降低残疾日志调用的成本。
	// Since Panic and higher may exit, we skip the optimization for those levels.
	//由于恐慌和较高的可能退出，我们会跳过对这些级别的优化。
	if lvl < zapcore.DPanicLevel && !log.core.Enabled(lvl) {
		return nil
	}

	// Create basic checked entry thru the core; this will be non-nil if the
	//通过核心创建基本的检查条目；如果这将是非尼尔的
	// log message will actually be written somewhere.
	//日志消息实际上将写在某个地方。
	ent := zapcore.Entry{
		LoggerName: log.name,
		Time:       log.clock.Now(),
		Level:      lvl,
		Message:    msg,
	}
	ce := log.core.Check(ent, nil)
	willWrite := ce != nil

	// Set up any required terminal behavior.
	//设置任何必需的终端行为。
	switch ent.Level {
	case zapcore.PanicLevel:
		ce = ce.Should(ent, zapcore.WriteThenPanic)
	case zapcore.FatalLevel:
		onFatal := log.onFatal
		// nil or WriteThenNoop will lead to continued execution after
		// nil或Writethennoop将导致持续执行
		// a Fatal log entry, which is unexpected. For example,
		//致命的日志条目，这是出乎意料的。例如，
		//
		//   f, err := os.Open(..)
		//   if err != nil {
		//     log.Fatal("cannot open", zap.Error(err))
		//   }
		//   fmt.Println(f.Name())
		//
		// The f.Name() will panic if we continue execution after the
		// log.Fatal.
		if onFatal == nil || onFatal == zapcore.WriteThenNoop {
			onFatal = zapcore.WriteThenFatal
		}
		ce = ce.After(ent, onFatal)
	case zapcore.DPanicLevel:
		if log.development {
			ce = ce.Should(ent, zapcore.WriteThenPanic)
		}
	}

	// Only do further annotation if we're going to write this message; checked
	// entries that exist only for terminal behavior don't benefit from
	// annotation.
	if !willWrite {
		return ce
	}

	// Thread the error output through to the CheckedEntry.
	//将错误输出通过到checkedentry。
	ce.ErrorOutput = log.errorOutput

	addStack := log.addStack.Enabled(ce.Level)
	if !log.addCaller && !addStack {
		return ce
	}

	// Adding the caller or stack trace requires capturing the callers of
	//添加呼叫者或堆栈跟踪需要捕获呼叫者
	//此功能。我们将在这两个之间分享信息。
	// this function. We'll share information between these two.
	stackDepth := stacktraceFirst
	if addStack {
		stackDepth = stacktraceFull
	}
	stack := captureStacktrace(log.callerSkip+callerSkipOffset, stackDepth)
	defer stack.Free()

	if stack.Count() == 0 {
		if log.addCaller {
			fmt.Fprintf(log.errorOutput, "%v Logger.check error: failed to get caller\n", ent.Time.UTC())
			log.errorOutput.Sync()
		}
		return ce
	}

	frame, more := stack.Next()

	if log.addCaller {
		ce.Caller = zapcore.EntryCaller{
			Defined:  frame.PC != 0,
			PC:       frame.PC,
			File:     frame.File,
			Line:     frame.Line,
			Function: frame.Function,
		}
	}

	if addStack {
		buffer := bufferpool.Get()
		defer buffer.Free()

		stackfmt := newStackFormatter(buffer)

		// We've already extracted the first frame, so format that
		//我们已经提取了第一帧，因此格式化
		// separately and defer to stackfmt for the rest.
		//单独推迟到stackfmt进行剩下的。
		stackfmt.FormatFrame(frame)
		if more {
			stackfmt.FormatStack(stack)
		}
		ce.Stack = buffer.String()
	}

	return ce
}
