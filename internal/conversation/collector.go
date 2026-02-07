// Package conversation 提供信息收集功能。
package conversation

import (
	"fmt"
	"strings"
)

// InfoType 定义需要收集的信息类型。
type InfoType string

const (
	InfoVersion   InfoType = "version"    // 版本信息
	InfoDevice    InfoType = "device"     // 设备信息
	InfoUser      InfoType = "user"       // 用户信息
	InfoIssue     InfoType = "issue"      // 问题描述
	InfoLogFile   InfoType = "logfile"    // 日志文件（可选）
)

// InfoConfig 定义信息类型的显示名称和提示语。
type InfoConfig struct {
	DisplayName string
	Prompt      string
	Examples    []string
}

// InfoConfigs 存储所有信息类型的配置。
var InfoConfigs = map[InfoType]InfoConfig{
	InfoVersion: {
		DisplayName: "版本信息",
		Prompt:      "请提供软件版本号",
		Examples:    []string{"v1.2.3", "2.0.1", "最新版"},
	},
	InfoDevice: {
		DisplayName: "设备信息",
		Prompt:      "请提供设备信息（如设备型号、操作系统等）",
		Examples:    []string{"iPhone 15 Pro / iOS 17", "Windows 11", "MacBook Pro M2"},
	},
	InfoUser: {
		DisplayName: "用户信息",
		Prompt:      "请提供您的用户信息（如姓名、工号等）",
		Examples:    []string{"张三", "工号12345"},
	},
	InfoIssue: {
		DisplayName: "问题描述",
		Prompt:      "请详细描述您遇到的问题",
		Examples:    []string{"登录时提示密码错误", "导出数据时程序崩溃"},
	},
	InfoLogFile: {
		DisplayName: "日志文件",
		Prompt:      "如有日志文件，可直接上传",
		Examples:    []string{},
	},
}

// RequiredInfos 必须收集的信息列表。
var RequiredInfos = []InfoType{InfoVersion, InfoDevice, InfoUser, InfoIssue}

// OptionalInfos 可选收集的信息列表。
var OptionalInfos = []InfoType{InfoLogFile}

// Collector 信息收集器，跟踪已收集的信息。
type Collector struct {
	collected map[InfoType]string
	fileKey   string // 日志文件的 fileKey
}

// NewCollector 创建新的信息收集器。
func NewCollector() *Collector {
	return &Collector{
		collected: make(map[InfoType]string),
	}
}

// Set 设置已收集的信息。
func (c *Collector) Set(infoType InfoType, value string) {
	c.collected[infoType] = strings.TrimSpace(value)
}

// SetFileKey 设置日志文件的 fileKey。
func (c *Collector) SetFileKey(fileKey string) {
	c.fileKey = fileKey
}

// Get 获取已收集的信息。
func (c *Collector) Get(infoType InfoType) (string, bool) {
	val, ok := c.collected[infoType]
	return val, ok
}

// HasFile 检查是否有日志文件。
func (c *Collector) HasFile() bool {
	return c.fileKey != ""
}

// GetFileKey 获取日志文件的 fileKey。
func (c *Collector) GetFileKey() string {
	return c.fileKey
}

// IsComplete 检查是否所有必须信息都已收集。
func (c *Collector) IsComplete() bool {
	for _, infoType := range RequiredInfos {
		if _, ok := c.collected[infoType]; !ok {
			return false
		}
	}
	return true
}

// GetMissing 获取缺失的必须信息列表。
func (c *Collector) GetMissing() []InfoType {
	var missing []InfoType
	for _, infoType := range RequiredInfos {
		if _, ok := c.collected[infoType]; !ok {
			missing = append(missing, infoType)
		}
	}
	return missing
}

// GetSummary 获取已收集信息的总结。
func (c *Collector) GetSummary() string {
	var sb strings.Builder

	sb.WriteString("===== 用户信息汇总 =====\n\n")

	// 必须信息
	for _, infoType := range RequiredInfos {
		config := InfoConfigs[infoType]
		if val, ok := c.collected[infoType]; ok {
			sb.WriteString(fmt.Sprintf("**%s**: %s\n", config.DisplayName, val))
		}
	}

	// 可选信息
	if c.fileKey != "" {
		sb.WriteString(fmt.Sprintf("**%s**: 已上传\n", InfoConfigs[InfoLogFile].DisplayName))
	}

	return sb.String()
}

// GetPrompt 获取当前需要提示用户的内容。
func (c *Collector) GetPrompt() string {
	missing := c.GetMissing()

	if len(missing) == 0 {
		return "信息已收集完整，正在为您转接人工..."
	}

	var sb strings.Builder
	sb.WriteString("为了更好地为您服务，请提供以下信息：\n\n")

	for i, infoType := range missing {
		config := InfoConfigs[infoType]
		sb.WriteString(fmt.Sprintf("%d. **%s**\n", i+1, config.DisplayName))
		if len(config.Examples) > 0 {
			sb.WriteString(fmt.Sprintf("   示例: %s\n", strings.Join(config.Examples, " / ")))
		}
	}

	sb.WriteString("\n您可以直接回复，例如：\n")
	sb.WriteString("- 版本：v1.2.3\n")
	sb.WriteString("- 设备：iPhone 15 Pro / iOS 17\n")

	return sb.String()
}

// ParseAndSet 解析用户输入并设置相应的信息类型。
func (c *Collector) ParseAndSet(input string) bool {
	input = strings.TrimSpace(input)
	if input == "" {
		return false
	}

	// 尝试解析各种格式的输入
	// 格式1: "版本：v1.2.3" 或 "version: v1.2.3"
	// 格式2: "v1.2.3" (尝试智能匹配)

	// 先检查是否包含明确的关键字
	lowerInput := strings.ToLower(input)

	// 检查版本信息
	if containsKeyword(lowerInput, []string{"版本", "version", "ver", "v"}) {
		value := extractValue(input)
		if value != "" {
			c.Set(InfoVersion, value)
			return true
		}
	}

	// 检查设备信息
	if containsKeyword(lowerInput, []string{"设备", "device", "ios", "android", "windows", "mac", "iphone", "ipad", "手机", "电脑"}) {
		value := extractValue(input)
		if value != "" {
			c.Set(InfoDevice, value)
			return true
		}
	}

	// 检查用户信息
	if containsKeyword(lowerInput, []string{"用户", "user", "我是", "我叫", "姓名", "工号"}) {
		value := extractValue(input)
		if value != "" {
			c.Set(InfoUser, value)
			return true
		}
	}

	// 检查问题描述（通常包含"问题"、"错误"、"失败"等关键词）
	if containsKeyword(lowerInput, []string{"问题", "problem", "error", "错误", "失败", "bug", "不能", "无法", "报错"}) {
		value := extractValue(input)
		if value != "" {
			c.Set(InfoIssue, value)
			return true
		}
	}

	// 如果没有明确关键字，尝试智能匹配
	// 如果输入很短且像版本号格式
	if isVersionLike(input) {
		c.Set(InfoVersion, input)
		return true
	}

	// 如果输入包含设备相关信息
	if isDeviceLike(input) {
		c.Set(InfoDevice, input)
		return true
	}

	// 默认作为问题描述
	return false
}

// containsKeyword 检查输入是否包含关键词。
func containsKeyword(input string, keywords []string) bool {
	for _, kw := range keywords {
		if strings.Contains(input, kw) {
			return true
		}
	}
	return false
}

// extractValue 从输入中提取值（去除常见的前缀）。
func extractValue(input string) string {
	// 去除常见的前缀
	prefixes := []string{"版本：", "版本:", "version:", "version：",
		"设备：", "设备:", "device:", "device：",
		"用户：", "用户:", "user:", "user：",
		"我是", "我叫", "姓名：", "姓名:"}

	value := input
	for _, prefix := range prefixes {
		if strings.HasPrefix(strings.ToLower(input), strings.ToLower(prefix)) {
			value = strings.TrimSpace(input[len(prefix):])
			break
		}
	}

	// 如果是 "我是xxx" 格式，取 "xxx" 部分
	if strings.HasPrefix(value, "是") {
		value = strings.TrimSpace(value[1:])
	}

	return value
}

// isVersionLike 检查输入是否像版本号。
func isVersionLike(input string) bool {
	// 匹配 v1.2.3, 1.2.3, 1.2 等格式
	return strings.HasPrefix(input, "v") ||
		(len(input) > 0 && input[0] >= '0' && input[0] <= '9')
}

// isDeviceLike 检查输入是否像设备信息。
func isDeviceLike(input string) bool {
	// 包含设备相关关键词
	keywords := []string{"iphone", "ipad", "android", "windows", "mac", "ios", "手机", "电脑", "pc"}
	lowerInput := strings.ToLower(input)
	for _, kw := range keywords {
		if strings.Contains(lowerInput, kw) {
			return true
		}
	}
	return false
}

// Reset 重置收集器。
func (c *Collector) Reset() {
	c.collected = make(map[InfoType]string)
	c.fileKey = ""
}

// ToMap 将收集到的信息转换为 map。
func (c *Collector) ToMap() map[InfoType]string {
	result := make(map[InfoType]string)
	for k, v := range c.collected {
		result[k] = v
	}
	return result
}
