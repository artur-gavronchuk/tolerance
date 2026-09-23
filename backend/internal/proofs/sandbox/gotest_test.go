package sandbox

import "testing"

func TestParseGoTestJSON(t *testing.T) {
	out := []byte(`{"Action":"run","Package":"retry","Test":"TestA"}
{"Action":"output","Package":"retry","Test":"TestA","Output":"=== RUN TestA\n"}
{"Action":"pass","Package":"retry","Test":"TestA","Elapsed":0}
{"Action":"run","Package":"retry","Test":"TestB"}
{"Action":"fail","Package":"retry","Test":"TestB","Elapsed":0.01}
{"Action":"fail","Package":"retry","Elapsed":0.02}
garbage line that is not json
`)
	got := ParseGoTestJSON(out)
	if len(got) != 2 || got[0].Name != "TestA" || !got[0].Passed || got[1].Name != "TestB" || got[1].Passed {
		t.Fatalf("unexpected: %+v", got)
	}
	if len(ParseGoTestJSON([]byte("# retry\n./retry.go:5: syntax error\n"))) != 0 {
		t.Fatalf("build failure yields no tests")
	}
}
