package voice

import (
	"testing"
)

func TestHandleIncomingCall(t *testing.T) {
	adapter := NewVoiceAdapter()

	t.Run("creates session with valid callerID", func(t *testing.T) {
		session, err := adapter.HandleIncomingCall("+1234567890")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if session == nil {
			t.Fatal("expected session, got nil")
		}
		if session.CallerID != "+1234567890" {
			t.Errorf("expected callerID +1234567890, got %s", session.CallerID)
		}
		if session.Status != SessionStatusActive {
			t.Errorf("expected status active, got %s", session.Status)
		}
		if session.SessionID == "" {
			t.Error("expected non-empty sessionID")
		}
		if session.StartTime.IsZero() {
			t.Error("expected non-zero start time")
		}
	})

	t.Run("rejects empty callerID", func(t *testing.T) {
		_, err := adapter.HandleIncomingCall("")
		if err == nil {
			t.Fatal("expected error for empty callerID")
		}
	})
}

func TestTranscribeAudio(t *testing.T) {
	adapter := NewVoiceAdapter()
	session, _ := adapter.HandleIncomingCall("+1234567890")

	t.Run("transcribes audio for active session", func(t *testing.T) {
		text, err := adapter.TranscribeAudio(session.SessionID, []byte("fake audio data"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if text == "" {
			t.Error("expected non-empty transcription")
		}
	})

	t.Run("rejects empty sessionID", func(t *testing.T) {
		_, err := adapter.TranscribeAudio("", []byte("audio"))
		if err == nil {
			t.Fatal("expected error for empty sessionID")
		}
	})

	t.Run("rejects empty audio", func(t *testing.T) {
		_, err := adapter.TranscribeAudio(session.SessionID, nil)
		if err == nil {
			t.Fatal("expected error for empty audio")
		}
	})

	t.Run("rejects unknown session", func(t *testing.T) {
		_, err := adapter.TranscribeAudio("nonexistent", []byte("audio"))
		if err == nil {
			t.Fatal("expected error for unknown session")
		}
	})
}

func TestSynthesizeSpeech(t *testing.T) {
	adapter := NewVoiceAdapter()
	session, _ := adapter.HandleIncomingCall("+1234567890")

	t.Run("synthesizes speech for active session", func(t *testing.T) {
		audio, err := adapter.SynthesizeSpeech(session.SessionID, "Hello world")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(audio) == 0 {
			t.Error("expected non-empty audio output")
		}
	})

	t.Run("rejects empty text", func(t *testing.T) {
		_, err := adapter.SynthesizeSpeech(session.SessionID, "")
		if err == nil {
			t.Fatal("expected error for empty text")
		}
	})

	t.Run("rejects unknown session", func(t *testing.T) {
		_, err := adapter.SynthesizeSpeech("nonexistent", "hello")
		if err == nil {
			t.Fatal("expected error for unknown session")
		}
	})
}

func TestEndSession(t *testing.T) {
	adapter := NewVoiceAdapter()

	t.Run("ends active session", func(t *testing.T) {
		session, _ := adapter.HandleIncomingCall("+1234567890")
		err := adapter.EndSession(session.SessionID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify session is ended.
		got, err := adapter.GetSession(session.SessionID)
		if err != nil {
			t.Fatalf("unexpected error getting session: %v", err)
		}
		if got.Status != SessionStatusEnded {
			t.Errorf("expected status ended, got %s", got.Status)
		}
	})

	t.Run("rejects ending already ended session", func(t *testing.T) {
		session, _ := adapter.HandleIncomingCall("+9876543210")
		_ = adapter.EndSession(session.SessionID)
		err := adapter.EndSession(session.SessionID)
		if err == nil {
			t.Fatal("expected error for already ended session")
		}
	})

	t.Run("rejects unknown session", func(t *testing.T) {
		err := adapter.EndSession("nonexistent")
		if err == nil {
			t.Fatal("expected error for unknown session")
		}
	})

	t.Run("rejects empty sessionID", func(t *testing.T) {
		err := adapter.EndSession("")
		if err == nil {
			t.Fatal("expected error for empty sessionID")
		}
	})
}

func TestSessionLifecycle(t *testing.T) {
	adapter := NewVoiceAdapter()

	// Create session.
	session, err := adapter.HandleIncomingCall("+1111111111")
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	// Transcribe while active.
	_, err = adapter.TranscribeAudio(session.SessionID, []byte("audio chunk 1"))
	if err != nil {
		t.Fatalf("transcribe failed on active session: %v", err)
	}

	// Synthesize while active.
	_, err = adapter.SynthesizeSpeech(session.SessionID, "response text")
	if err != nil {
		t.Fatalf("synthesize failed on active session: %v", err)
	}

	// End session.
	err = adapter.EndSession(session.SessionID)
	if err != nil {
		t.Fatalf("end session failed: %v", err)
	}

	// Operations should fail on ended session.
	_, err = adapter.TranscribeAudio(session.SessionID, []byte("more audio"))
	if err == nil {
		t.Error("expected error transcribing on ended session")
	}

	_, err = adapter.SynthesizeSpeech(session.SessionID, "more text")
	if err == nil {
		t.Error("expected error synthesizing on ended session")
	}
}

func TestVoiceAdapterOptions(t *testing.T) {
	adapter := NewVoiceAdapter(
		WithSTTProvider("azure-speech"),
		WithTTSProvider("elevenlabs"),
	)

	if adapter.sttProvider != "azure-speech" {
		t.Errorf("expected sttProvider azure-speech, got %s", adapter.sttProvider)
	}
	if adapter.ttsProvider != "elevenlabs" {
		t.Errorf("expected ttsProvider elevenlabs, got %s", adapter.ttsProvider)
	}
}
