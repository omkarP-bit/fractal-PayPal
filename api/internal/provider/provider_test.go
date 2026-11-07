package provider

import "testing"

func TestWebhookHeadersComplete(t *testing.T) {
	tests := []struct {
		name   string
		h      WebhookHeaders
		expect bool
	}{
		{
			name: "all headers present",
			h: WebhookHeaders{
				TransmissionID:   "tid",
				TransmissionTime: "tt",
				TransmissionSig:  "sig",
				CertURL:          "https://cert",
				AuthAlgo:         "SHA256withRSA",
			},
			expect: true,
		},
		{
			name: "missing transmission id",
			h: WebhookHeaders{
				TransmissionTime: "tt",
				TransmissionSig:  "sig",
				CertURL:          "https://cert",
				AuthAlgo:         "SHA256withRSA",
			},
			expect: false,
		},
		{
			name: "missing transmission time",
			h: WebhookHeaders{
				TransmissionID:   "tid",
				TransmissionSig:  "sig",
				CertURL:          "https://cert",
				AuthAlgo:         "SHA256withRSA",
			},
			expect: false,
		},
		{
			name: "missing transmission sig",
			h: WebhookHeaders{
				TransmissionID:   "tid",
				TransmissionTime: "tt",
				CertURL:          "https://cert",
				AuthAlgo:         "SHA256withRSA",
			},
			expect: false,
		},
		{
			name: "missing cert url",
			h: WebhookHeaders{
				TransmissionID:   "tid",
				TransmissionTime: "tt",
				TransmissionSig:  "sig",
				AuthAlgo:         "SHA256withRSA",
			},
			expect: false,
		},
		{
			name: "missing auth algo",
			h: WebhookHeaders{
				TransmissionID:   "tid",
				TransmissionTime: "tt",
				TransmissionSig:  "sig",
				CertURL:          "https://cert",
			},
			expect: false,
		},
		{
			name:   "empty headers",
			h:      WebhookHeaders{},
			expect: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.h.Complete()
			if got != tt.expect {
				t.Errorf("Complete() = %v, want %v", got, tt.expect)
			}
		})
	}
}
