package rpc

import (
	"errors"
	"testing"
)

func TestConfig_Validate(t *testing.T) {
	// Define test cases
	tests := []struct {
		name          string
		config        Config
		expectedError error
	}{
		{
			name: "incomplete server certificate configuration",
			config: Config{
				ServerKey:        "key.pem",
				ServerCert:       "",
				ServerSkipVerify: false,
				ServerCAs:        []string{},
				ClientKey:        "",
				ClientCert:       "",
				ClientSkipVerify: false,
				ClientCAs:        []string{},
			},
			expectedError: errors.New("incomplete server certificate configuration"),
		},
		{
			name: "no server CAs configured",
			config: Config{
				ServerKey:        "cert.key",
				ServerCert:       "cert.pem",
				ServerSkipVerify: false,
				ServerCAs:        []string{},
				ClientKey:        "",
				ClientCert:       "",
				ClientSkipVerify: false,
				ClientCAs:        []string{},
			},
			expectedError: errors.New("no server CAs configured"),
		},
		{
			name: "incomplete client certificate configuration",
			config: Config{
				ServerKey:        "",
				ServerCert:       "",
				ServerSkipVerify: false,
				ServerCAs:        []string{},
				ClientKey:        "key.pem",
				ClientCert:       "",
				ClientSkipVerify: false,
				ClientCAs:        []string{},
			},
			expectedError: errors.New("incomplete client certificate configuration"),
		},
		{
			name: "no client CAs configured",
			config: Config{
				ServerKey:        "",
				ServerCert:       "",
				ServerSkipVerify: false,
				ServerCAs:        []string{},
				ClientKey:        "cert.key",
				ClientCert:       "cert.pem",
				ClientSkipVerify: false,
				ClientCAs:        []string{},
			},
			expectedError: errors.New("no client CAs configured"),
		},
		{
			name: "valid configuration",
			config: Config{
				ServerKey:        "key.pem",
				ServerCert:       "cert.pem",
				ServerSkipVerify: true,
				ServerCAs:        []string{},
				ClientKey:        "client_key.pem",
				ClientCert:       "client_cert.pem",
				ClientSkipVerify: true,
				ClientCAs:        []string{},
			},
			expectedError: nil,
		},
		{
			name: "empty configuration",
			config: Config{
				ServerKey:        "",
				ServerCert:       "",
				ServerSkipVerify: false,
				ServerCAs:        []string{},
				ClientKey:        "",
				ClientCert:       "",
				ClientSkipVerify: false,
				ClientCAs:        []string{},
			},
			expectedError: nil,
		},
	}

	// Run test cases
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()

			if tt.expectedError != nil && err == nil {
				t.Errorf("expected error %v, but got nil", tt.expectedError)
			}

			if tt.expectedError == nil && err != nil {
				t.Errorf("expected no error, but got %v", err)
			}

			if tt.expectedError != nil && err != nil && tt.expectedError.Error() != err.Error() {
				t.Errorf("expected error %v, but got %v", tt.expectedError, err)
			}
		})
	}
}
