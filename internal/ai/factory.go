
package ai

import "messenger/internal/config"

func New(cfg config.AIConfig) LLMClient {
    if cfg.OllamaURL == "" {
        // Нет URL — используем заглушку
        return NewStubLLM()
    }
    return NewOllamaLLM(
        cfg.OllamaURL,
        cfg.SmartReplyModel,
        cfg.SummaryModel,
        cfg.MaxPromptChars,
        cfg.RequestTimeout,
    )
}