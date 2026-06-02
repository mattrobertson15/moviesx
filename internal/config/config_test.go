package config

import (
	"os"
	"testing"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name     string
		env      map[string]string
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
			name:     "overrides",
			env:      map[string]string{"MOVIES_PORT": "9090", "MOVIES_LOG_LEVEL": "debug", "MOVIES_DATA_DIR": "/tmp/data"},
			wantPort: "9090",
			wantLog:  "debug",
			wantData: "/tmp/data",
		},
		{
			name:     "partial override",
			env:      map[string]string{"MOVIES_PORT": "3000"},
			wantPort: "3000",
			wantLog:  "info",
			wantData: "/data",
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

			cfg := Load()
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
