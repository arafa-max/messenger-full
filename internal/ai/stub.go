
package ai

import "context"

type StubLLM struct{}

func NewStubLLM() LLMClient {
    return &StubLLM{}
}

func (s *StubLLM) SmartReply(_ context.Context, _ []Message) ([]string, error) {
    return []string{"Окей", "Понял", "Позже напишу"}, nil
}

func (s *StubLLM) Summarize(_ context.Context, messages []Message) (string, error) {
    return "Резюме временно недоступно.", nil
}

func (s *StubLLM) Moderate(_ context.Context, _ string) (ModResult, error) {
    // Заглушка — пропускаем всё
    return ModResult{IsSpam: false, IsToxic: false, Score: 0}, nil
}