// Package conversation provides prompt management for the bot.
package conversation

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

// PromptManager manages system prompts for different scenarios.
type PromptManager struct {
	prompts map[string]string
}

// NewPromptManager creates a new prompt manager.
func NewPromptManager(promptsPath string) (*PromptManager, error) {
	pm := &PromptManager{
		prompts: make(map[string]string),
	}

	if err := pm.loadFromFile(promptsPath); err != nil {
		// If file loading fails, use default prompts
		pm.loadDefaultPrompts()
		return pm, nil
	}

	return pm, nil
}

// loadFromFile loads prompts from a YAML file.
func (pm *PromptManager) loadFromFile(path string) error {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")

	if err := v.ReadInConfig(); err != nil {
		return err
	}

	prompts := v.GetStringMapString("prompts")
	for name, prompt := range prompts {
		pm.prompts[name] = strings.TrimSpace(prompt)
	}

	return nil
}

// loadDefaultPrompts loads default fallback prompts.
func (pm *PromptManager) loadDefaultPrompts() {
	pm.prompts["default"] = `你是公司的智能助手，可以帮助员工解决日常工作问题。
请用简洁、专业的语言回答问题。如果遇到无法解决的问题，请告知用户可以发送"转人工"获取帮助。`
}

// GetPrompt returns the prompt for the given scenario.
func (pm *PromptManager) GetPrompt(scenario string) string {
	if prompt, ok := pm.prompts[scenario]; ok {
		return prompt
	}
	return pm.prompts["default"]
}

// SetPrompt sets a prompt for the given scenario.
func (pm *PromptManager) SetPrompt(scenario, prompt string) {
	pm.prompts[scenario] = strings.TrimSpace(prompt)
}

// ListPrompts returns all available prompt names.
func (pm *PromptManager) ListPrompts() []string {
	names := make([]string, 0, len(pm.prompts))
	for name := range pm.prompts {
		names = append(names, name)
	}
	return names
}

// FormatSystemPrompt formats a system prompt with additional context.
func (pm *PromptManager) FormatSystemPrompt(scenario string, context map[string]string) string {
	prompt := pm.GetPrompt(scenario)

	// Replace placeholders in the prompt
	for key, value := range context {
		placeholder := fmt.Sprintf("{{%s}}", key)
		prompt = strings.ReplaceAll(prompt, placeholder, value)
	}

	return prompt
}
