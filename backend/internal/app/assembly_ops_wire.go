//go:build wireinject

package app

import (
	"github.com/google/wire"
)

// opsAssemblyProviders 汇总 ops 模块的 Wire provider。
var opsAssemblyProviders = wire.NewSet(
	provideChannelMonitor,
	provideSystemLock,
	provideSystemOperations,
	provideSystemHTTP,
	provideOpsErrorQueue,
	opsProviders,
	provideSystemLogSink,
)
