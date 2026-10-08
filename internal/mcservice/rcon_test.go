package mcservice

import (
	"reflect"
	"testing"
)

func TestParsePlayerList(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantPlayers []string
		wantOnline  int
		wantMax     int
		wantErr     bool
	}{
		{
			name:        "Multiple players online",
			input:       "There are 2 of a max of 20 players online: Steve, Alex",
			wantPlayers: []string{"Steve", "Alex"},
			wantOnline:  2,
			wantMax:     20,
			wantErr:     false,
		},
		{
			name:        "No players online",
			input:       "There are 0 of a max of 20 players online:",
			wantPlayers: nil,
			wantOnline:  0,
			wantMax:     20,
			wantErr:     false,
		},
		{
			name:        "Alternative phrasing without colon suffix",
			input:       "There are 0 of a max of 10 players online",
			wantPlayers: nil,
			wantOnline:  0,
			wantMax:     10,
			wantErr:     false,
		},
		{
			name:        "Single player with whitespace",
			input:       "There are 1 of a max of 5 players online:  Gamer_123  ",
			wantPlayers: []string{"Gamer_123"},
			wantOnline:  1,
			wantMax:     5,
			wantErr:     false,
		},
		{
			name:        "Malformed output",
			input:       "Unknown command or unexpected text",
			wantPlayers: nil,
			wantOnline:  0,
			wantMax:     0,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPlayers, gotOnline, gotMax, err := ParsePlayerList(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParsePlayerList() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr {
				if gotOnline != tt.wantOnline {
					t.Errorf("gotOnline = %d, want %d", gotOnline, tt.wantOnline)
				}
				if gotMax != tt.wantMax {
					t.Errorf("gotMax = %d, want %d", gotMax, tt.wantMax)
				}
				if !reflect.DeepEqual(gotPlayers, tt.wantPlayers) {
					t.Errorf("gotPlayers = %v, want %v", gotPlayers, tt.wantPlayers)
				}
			}
		})
	}
}
