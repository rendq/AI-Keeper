// Package voice implements the Voice channel adapter for AIP Gateway.
// It provides SIP/WebRTC session management with speech-to-text and
// text-to-speech stubs for integration with external ASR/TTS providers.
package voice

import (
	"errors"
	"sync"
	"time"
)

// SessionStatus represents the current state of a voice session.
type SessionStatus string

const (
	// SessionStatusActive indicates an active voice session.
	SessionStatusActive SessionStatus = "active"
	// SessionStatusEnded indicates a terminated voice session.
	SessionStatusEnded SessionStatus = "ended"
)

// VoiceSession represents an active voice call session.
type VoiceSession struct {
	SessionID string
	CallerID  string
	Status    SessionStatus
	StartTime time.Time
}

// VoiceAdapter handles SIP/WebRTC voice sessions with STT/TTS capabilities.
// This is a stub implementation defining the interface for future provider integration.
type VoiceAdapter struct {
	mu       sync.RWMutex
	sessions map[string]*VoiceSession

	// Configuration for external providers (stub).
	sttProvider string // e.g., "whisper", "azure-speech"
	ttsProvider string // e.g., "azure-tts", "elevenlabs"
}

// Option configures a VoiceAdapter.
type Option func(*VoiceAdapter)

// WithSTTProvider sets the speech-to-text provider.
func WithSTTProvider(provider string) Option {
	return func(a *VoiceAdapter) { a.sttProvider = provider }
}

// WithTTSProvider sets the text-to-speech provider.
func WithTTSProvider(provider string) Option {
	return func(a *VoiceAdapter) { a.ttsProvider = provider }
}

// NewVoiceAdapter creates a new Voice channel adapter.
func NewVoiceAdapter(opts ...Option) *VoiceAdapter {
	a := &VoiceAdapter{
		sessions:    make(map[string]*VoiceSession),
		sttProvider: "whisper",
		ttsProvider: "azure-tts",
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// HandleIncomingCall creates a new voice session for an incoming call.
func (a *VoiceAdapter) HandleIncomingCall(callerID string) (*VoiceSession, error) {
	if callerID == "" {
		return nil, errors.New("voice: callerID is required")
	}

	sessionID := generateSessionID()

	session := &VoiceSession{
		SessionID: sessionID,
		CallerID:  callerID,
		Status:    SessionStatusActive,
		StartTime: time.Now(),
	}

	a.mu.Lock()
	a.sessions[sessionID] = session
	a.mu.Unlock()

	return session, nil
}

// TranscribeAudio converts audio bytes to text using the configured STT provider.
// This is a stub implementation that returns a placeholder transcription.
func (a *VoiceAdapter) TranscribeAudio(sessionID string, audio []byte) (string, error) {
	if sessionID == "" {
		return "", errors.New("voice: sessionID is required")
	}
	if len(audio) == 0 {
		return "", errors.New("voice: audio data is empty")
	}

	a.mu.RLock()
	session, ok := a.sessions[sessionID]
	a.mu.RUnlock()

	if !ok {
		return "", errors.New("voice: session not found")
	}
	if session.Status != SessionStatusActive {
		return "", errors.New("voice: session is not active")
	}

	// Stub: In production, this would call the configured STT provider
	// (e.g., OpenAI Whisper, Azure Speech Services, Google Cloud Speech).
	return "[transcribed audio placeholder]", nil
}

// SynthesizeSpeech converts text to audio using the configured TTS provider.
// This is a stub implementation that returns placeholder audio bytes.
func (a *VoiceAdapter) SynthesizeSpeech(sessionID, text string) ([]byte, error) {
	if sessionID == "" {
		return nil, errors.New("voice: sessionID is required")
	}
	if text == "" {
		return nil, errors.New("voice: text is required")
	}

	a.mu.RLock()
	session, ok := a.sessions[sessionID]
	a.mu.RUnlock()

	if !ok {
		return nil, errors.New("voice: session not found")
	}
	if session.Status != SessionStatusActive {
		return nil, errors.New("voice: session is not active")
	}

	// Stub: In production, this would call the configured TTS provider
	// (e.g., Azure TTS, ElevenLabs, Google Cloud TTS).
	return []byte("audio:" + text), nil
}

// EndSession terminates an active voice session.
func (a *VoiceAdapter) EndSession(sessionID string) error {
	if sessionID == "" {
		return errors.New("voice: sessionID is required")
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	session, ok := a.sessions[sessionID]
	if !ok {
		return errors.New("voice: session not found")
	}
	if session.Status == SessionStatusEnded {
		return errors.New("voice: session already ended")
	}

	session.Status = SessionStatusEnded
	return nil
}

// GetSession retrieves a voice session by ID.
func (a *VoiceAdapter) GetSession(sessionID string) (*VoiceSession, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	session, ok := a.sessions[sessionID]
	if !ok {
		return nil, errors.New("voice: session not found")
	}
	return session, nil
}

// generateSessionID produces a unique session identifier.
func generateSessionID() string {
	return "vs-" + time.Now().Format("20060102150405") + "-" + randomSuffix()
}

// randomSuffix generates a short random string for session ID uniqueness.
func randomSuffix() string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 8)
	for i := range b {
		b[i] = charset[time.Now().UnixNano()%int64(len(charset))]
		time.Sleep(time.Nanosecond)
	}
	return string(b)
}
