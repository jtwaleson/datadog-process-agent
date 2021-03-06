package config

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func setupDataScrubber(t *testing.T) *DataScrubber {
	customSensitiveWords := []string{
		"consul_token",
		"dd_password",
		"blocked_from_yaml",
	}
	expectedPatterns := make([]string, 0, len(defaultSensitiveWords)+len(customSensitiveWords))
	for _, word := range append(defaultSensitiveWords, customSensitiveWords...) {
		expectedPatterns = append(expectedPatterns, `(?P<key>( |-)(?i)`+word+`)(?P<delimiter> +|=)(?P<value>[^\s]*)`)
	}

	scrubber := NewDefaultDataScrubber()
	scrubber.AddCustomSensitiveWords(customSensitiveWords)

	assert.Equal(t, true, scrubber.Enabled)
	for i, pattern := range scrubber.SensitivePatterns {
		assert.Equal(t, expectedPatterns[i], fmt.Sprint(pattern))
	}

	return scrubber
}

func TestUncompilableWord(t *testing.T) {
	customSensitiveWords := []string{
		"consul_token",
		"dd_password",
		"(an_error",
		")a*",
		"[forbidden]",
		"]a*",
		"blocked_from_yaml",
	}

	validCustomSenstiveWords := []string{
		"consul_token",
		"dd_password",
		"blocked_from_yaml",
	}

	expectedPatterns := make([]string, 0, len(defaultSensitiveWords)+len(validCustomSenstiveWords))
	for _, word := range append(defaultSensitiveWords, validCustomSenstiveWords...) {
		expectedPatterns = append(expectedPatterns, `(?P<key>( |-)(?i)`+word+`)(?P<delimiter> +|=)(?P<value>[^\s]*)`)
	}

	scrubber := NewDefaultDataScrubber()
	scrubber.AddCustomSensitiveWords(customSensitiveWords)

	assert.Equal(t, true, scrubber.Enabled)
	for i, pattern := range scrubber.SensitivePatterns {
		assert.Equal(t, expectedPatterns[i], fmt.Sprint(pattern))
	}
}

func TestBlacklistedArgs(t *testing.T) {
	cases := []struct {
		cmdline       []string
		parsedCmdline []string
	}{
		{[]string{"agent", "-password", "1234"}, []string{"agent", "-password", "********"}},
		{[]string{"agent", "--password", "1234"}, []string{"agent", "--password", "********"}},
		{[]string{"agent", "-password=1234"}, []string{"agent", "-password=********"}},
		{[]string{"agent", "--password=1234"}, []string{"agent", "--password=********"}},
		{[]string{"fitz", "-consul_token=1234567890"}, []string{"fitz", "-consul_token=********"}},
		{[]string{"fitz", "--consul_token=1234567890"}, []string{"fitz", "--consul_token=********"}},
		{[]string{"fitz", "-consul_token", "1234567890"}, []string{"fitz", "-consul_token", "********"}},
		{[]string{"fitz", "--consul_token", "1234567890"}, []string{"fitz", "--consul_token", "********"}},
		{[]string{"python ~/test/run.py --password=1234 -password 1234 -open_password=admin -consul_token 2345 -blocked_from_yaml=1234 &"},
			[]string{"python", "~/test/run.py", "--password=********", "-password", "********", "-open_password=admin", "-consul_token", "********", "-blocked_from_yaml=********", "&"}},
		{[]string{"agent", "-PASSWORD", "1234"}, []string{"agent", "-PASSWORD", "********"}},
		{[]string{"agent", "--PASSword", "1234"}, []string{"agent", "--PASSword", "********"}},
		{[]string{"agent", "--PaSsWoRd=1234"}, []string{"agent", "--PaSsWoRd=********"}},
		{[]string{"java -password      1234"}, []string{"java", "-password", "", "", "", "", "", "********"}},
	}

	scrubber := setupDataScrubber(t)

	for i := range cases {
		cases[i].cmdline = scrubber.ScrubCmdline(cases[i].cmdline)
		assert.Equal(t, cases[i].parsedCmdline, cases[i].cmdline)
	}
}

func TestBlacklistedArgsWhenDisabled(t *testing.T) {
	cases := []struct {
		cmdline       []string
		parsedCmdline []string
	}{
		{[]string{"agent", "-password", "1234"}, []string{"agent", "-password", "1234"}},
		{[]string{"agent", "--password", "1234"}, []string{"agent", "--password", "1234"}},
		{[]string{"agent", "-password=1234"}, []string{"agent", "-password=1234"}},
		{[]string{"agent", "--password=1234"}, []string{"agent", "--password=1234"}},
		{[]string{"fitz", "-consul_token=1234567890"}, []string{"fitz", "-consul_token=1234567890"}},
		{[]string{"fitz", "--consul_token=1234567890"}, []string{"fitz", "--consul_token=1234567890"}},
		{[]string{"fitz", "-consul_token", "1234567890"}, []string{"fitz", "-consul_token", "1234567890"}},
		{[]string{"fitz", "--consul_token", "1234567890"}, []string{"fitz", "--consul_token", "1234567890"}},
		{[]string{"python ~/test/run.py --password=1234 -password 1234 -open_password=admin -consul_token 2345 -blocked_from_yaml=1234 &"},
			[]string{"python ~/test/run.py --password=1234 -password 1234 -open_password=admin -consul_token 2345 -blocked_from_yaml=1234 &"}},
		{[]string{"agent", "-PASSWORD", "1234"}, []string{"agent", "-PASSWORD", "1234"}},
		{[]string{"agent", "--PASSword", "1234"}, []string{"agent", "--PASSword", "1234"}},
		{[]string{"agent", "--PaSsWoRd=1234"}, []string{"agent", "--PaSsWoRd=1234"}},
		{[]string{"java -password      1234"}, []string{"java -password      1234"}},
	}

	scrubber := setupDataScrubber(t)
	scrubber.Enabled = false

	for i := range cases {
		cases[i].cmdline = scrubber.ScrubCmdline(cases[i].cmdline)
		assert.Equal(t, cases[i].parsedCmdline, cases[i].cmdline)
	}
}

func TestNoBlacklistedArgs(t *testing.T) {
	cases := []struct {
		cmdline       []string
		parsedCmdline []string
	}{
		{[]string{"spidly", "--debug_port=2043"}, []string{"spidly", "--debug_port=2043"}},
		{[]string{"agent", "start", "-p", "config.cfg"}, []string{"agent", "start", "-p", "config.cfg"}},
		{[]string{"p1", "--openpassword=admin"}, []string{"p1", "--openpassword=admin"}},
		{[]string{"p1", "-openpassword", "admin"}, []string{"p1", "-openpassword", "admin"}},
		{[]string{"java -openpassword 1234"}, []string{"java -openpassword 1234"}},
		{[]string{"java -open_password 1234"}, []string{"java -open_password 1234"}},
		{[]string{"java -passwordOpen 1234"}, []string{"java -passwordOpen 1234"}},
		{[]string{"java -password_open 1234"}, []string{"java -password_open 1234"}},
		{[]string{"java -password1 1234"}, []string{"java -password1 1234"}},
		{[]string{"java -password_1 1234"}, []string{"java -password_1 1234"}},
		{[]string{"java -1password 1234"}, []string{"java -1password 1234"}},
		{[]string{"java -1_password 1234"}, []string{"java -1_password 1234"}},
	}

	scrubber := setupDataScrubber(t)

	for i := range cases {
		cases[i].cmdline = scrubber.ScrubCmdline(cases[i].cmdline)
		assert.Equal(t, cases[i].parsedCmdline, cases[i].cmdline)
	}

}

func BenchmarkRegexMatching1(b *testing.B)    { benchmarkRegexMatching(1, b) }
func BenchmarkRegexMatching10(b *testing.B)   { benchmarkRegexMatching(10, b) }
func BenchmarkRegexMatching100(b *testing.B)  { benchmarkRegexMatching(100, b) }
func BenchmarkRegexMatching1000(b *testing.B) { benchmarkRegexMatching(1000, b) }

var avoidOptimization []string

func benchmarkRegexMatching(nbProcesses int, b *testing.B) {
	runningProcesses := make([][]string, nbProcesses)
	foolCmdline := []string{"python ~/test/run.py --password=1234 -password 1234 -password=admin -secret 2345 -credentials=1234 -api_key 2808 &"}

	customSensitiveWords := []string{
		"consul_token",
		"dd_password",
		"blocked_from_yaml",
	}
	scrubber := NewDefaultDataScrubber()
	scrubber.AddCustomSensitiveWords(customSensitiveWords)

	for i := 0; i < nbProcesses; i++ {
		runningProcesses = append(runningProcesses, foolCmdline)
	}

	var r []string
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		for _, p := range runningProcesses {
			r = scrubber.ScrubCmdline(p)
		}
	}

	avoidOptimization = r
}
