package main

import (
	"fmt"
	"sync"
)

// MockProvider implements Provider interface for testing
type MockProvider struct {
	mu       sync.Mutex
	messages []struct {
		message *GoogleChatMessage
		reqID   string
	}
	shouldFail bool
}

func NewMockProvider(shouldFail bool) *MockProvider {
	return &MockProvider{
		messages: make([]struct {
			message *GoogleChatMessage
			reqID   string
		}, 0),
		shouldFail: shouldFail,
	}
}

func (m *MockProvider) Send(message *GoogleChatMessage, reqID string) error {
	if m.shouldFail {
		return fmt.Errorf("mock provider configured to fail")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.messages = append(m.messages, struct {
		message *GoogleChatMessage
		reqID   string
	}{message, reqID})

	return nil
}

func (m *MockProvider) GetSentMessages() []struct {
	message *GoogleChatMessage
	reqID   string
} {
	m.mu.Lock()
	defer m.mu.Unlock()

	return append([]struct {
		message *GoogleChatMessage
		reqID   string
	}{}, m.messages...)
}
