package utils

import "strings"

// sanitizeFileName 清理文件名
func SanitizeFileName(name string) string {
	// 替换不允许在文件名中使用的字符
	invalid := []rune{'<', '>', ':', '"', '/', '\\', '|', '?', '*'}
	for _, r := range invalid {
		name = strings.ReplaceAll(name, string(r), "_")
	}
	return name
}
