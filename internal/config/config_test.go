package config

import (
	"os"
	"testing"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name     string
		env      map[string]string
		flags    Flags
		wantPort string
		wantLog  string
		wantData string
	}{
		{
			name:     "defaults",
			env:      nil,
			wantPort: "8080",
			wantLog:  "info",
			wantData: "/data",
		},
		{
			name:     "env overrides",
			env:      map[string]string{"MOVIES_PORT": "9090", "MOVIES_LOG_LEVEL": "debug", "MOVIES_DATA_DIR": "/tmp/data"},
			wantPort: "9090",
			wantLog:  "debug",
			wantData: "/tmp/data",
		},
		{
			name:     "partial env override",
			env:      map[string]string{"MOVIES_PORT": "3000"},
			wantPort: "3000",
			wantLog:  "info",
			wantData: "/data",
		},
		{
			name:     "flags override env",
			env:      map[string]string{"MOVIES_PORT": "9090"},
			flags:    Flags{Port: "7777"},
			wantPort: "7777",
			wantLog:  "info",
			wantData: "/data",
		},
		{
			name:     "flags override defaults",
			flags:    Flags{Port: "5000", LogLevel: "debug", DataDir: "/tmp"},
			wantPort: "5000",
			wantLog:  "debug",
			wantData: "/tmp",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			os.Unsetenv("MOVIES_PORT")
			os.Unsetenv("MOVIES_LOG_LEVEL")
			os.Unsetenv("MOVIES_DATA_DIR")

			for k, v := range tc.env {
				os.Setenv(k, v)
			}
			t.Cleanup(func() {
				os.Unsetenv("MOVIES_PORT")
				os.Unsetenv("MOVIES_LOG_LEVEL")
				os.Unsetenv("MOVIES_DATA_DIR")
			})

			cfg := Load(tc.flags)
			if cfg.Port != tc.wantPort {
				t.Errorf("Port = %q, want %q", cfg.Port, tc.wantPort)
			}
			if cfg.LogLevel != tc.wantLog {
				t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, tc.wantLog)
			}
			if cfg.DataDir != tc.wantData {
				t.Errorf("DataDir = %q, want %q", cfg.DataDir, tc.wantData)
			}
		})
	}
}
