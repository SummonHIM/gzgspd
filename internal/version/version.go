// Package version 保存构建期注入的版本信息。
package version

import "fmt"

var (
	// Value 由构建期 -ldflags -X 注入。
	Value = "dev"
	// BuildTime 由构建期 -ldflags -X 注入。
	BuildTime = "0"
)

// String 返回 "Value BuildTime" 形式，供 CLI 与 GUI 展示。
func String() string {
	return fmt.Sprintf("%s %s", Value, BuildTime)
}
