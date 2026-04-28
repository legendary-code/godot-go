package naming

import "testing"

func TestPascalToSnake(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"X", "x"},
		{"Hello", "hello"},
		{"HelloWorld", "hello_world"},
		{"HTTPServer", "http_server"},     // acronym at start
		{"GetURL", "get_url"},             // acronym at end
		{"ParseHTMLString", "parse_html_string"}, // acronym in middle
		{"Vector2", "vector2"},            // trailing digits stay attached
		{"Vector2D", "vector2_d"},         // digit-then-uppercase splits
		{"AudioStreamPlayer2D", "audio_stream_player2_d"},
		{"OAuth2", "o_auth2"},             // OAuth2 isn't a clean conversion — acronym detection is naive
		{"_process", "_process"},          // already snake-ish, leading underscore preserved
		{"DoSomething", "do_something"},
	}
	for _, c := range cases {
		if got := PascalToSnake(c.in); got != c.want {
			t.Errorf("PascalToSnake(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSnakeToPascal(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"x", "X"},
		{"hello", "Hello"},
		{"hello_world", "HelloWorld"},
		{"http_server", "HttpServer"}, // acronym round-trip lossy — by design
		{"get_url", "GetUrl"},
		{"vector2", "Vector2"},
		{"audio_stream_player_2d", "AudioStreamPlayer2d"},
		{"_process", "Process"}, // leading underscore drops a leading empty segment, then capitalizes
		{"a_b_c", "ABC"},
	}
	for _, c := range cases {
		if got := SnakeToPascal(c.in); got != c.want {
			t.Errorf("SnakeToPascal(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestRoundTripCommonCase confirms the typical user identifier
// (no acronyms) survives a round trip cleanly. Acronyms are
// inherently lossy in snake_case and not part of this guarantee.
func TestRoundTripCommonCase(t *testing.T) {
	roundTrips := []string{
		"Hello",
		"HelloWorld",
		"GetSomething",
		"Position",
		"Damage",
	}
	for _, s := range roundTrips {
		snake := PascalToSnake(s)
		back := SnakeToPascal(snake)
		if back != s {
			t.Errorf("round trip %q → %q → %q (lost identity)", s, snake, back)
		}
	}
}
