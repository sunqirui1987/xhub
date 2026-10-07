// Package all imports every provider directory so their endpoint types register.
// Adding a supplier is a new directory plus one import line in this file.
package all

import (
	_ "github.com/sunqirui1987/xhub/internal/provider/openai"
	_ "github.com/sunqirui1987/xhub/internal/provider/qiniu"
	_ "github.com/sunqirui1987/xhub/internal/provider/volcengine"
)
