package detector

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestParseLine(t *testing.T) {
	name, pid, in, out, header, ok := parseLine("UURemoteServer.3268,381370,297154,\r")
	if !ok || header || name != "UURemoteServer" || pid != 3268 || in != 381370 || out != 297154 {
		t.Fatalf("got %q %d %d %d %v %v", name, pid, in, out, header, ok)
	}
	if _, _, _, _, header, _ := parseLine(",bytes_in,bytes_out,"); !header {
		t.Fatal("header not detected")
	}
	if _, _, _, _, _, ok := parseLine("garbage"); ok {
		t.Fatal("garbage parsed")
	}
	// Names may contain dots; the pid is after the last one.
	if name, pid, _, _, _, ok := parseLine("com.apple.Web.12,1,2,"); !ok || name != "com.apple.Web" || pid != 12 {
		t.Fatalf("dotted name: %q %d", name, pid)
	}
}

func TestReadSamplesComputesUploadOfMatchingProcesses(t *testing.T) {
	input := strings.Join([]string{
		",bytes_in,bytes_out,",
		"UURemote.10,1000,1000,",
		"UURemoteServer.20,0,0,",
		"Safari.30,0,0,",
		",bytes_in,bytes_out,",
		"UURemote.10,1000,1000,",
		"UURemoteServer.20,2048,204800,",
		"Safari.30,0,99999999,",
		",bytes_in,bytes_out,",
	}, "\n")
	clock := time.Unix(100, 0)
	now := func() time.Time { clock = clock.Add(time.Second); return clock }
	ch := make(chan Sample, 4)
	_ = readSamples(context.Background(), strings.NewReader(input),
		func() []string { return []string{"UURemote"} }, func() int { return 3 }, ch, now)
	close(ch)
	var got []Sample
	for s := range ch {
		got = append(got, s)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 sample, got %d", len(got))
	}
	if got[0].UploadKBps != 200 || got[0].DownKBps != 2 || got[0].Processes != 2 || got[0].Conns != 3 {
		t.Fatalf("bad sample %+v", got[0])
	}
}

func TestCountConnLines(t *testing.T) {
	out := `COMMAND    PID USER   FD   TYPE DEVICE SIZE/OFF NODE NAME
UURemote  3158   4b   85u  IPv4 0x1      0t0  TCP 198.19.0.1:54436->198.0.2.164:443 (ESTABLISHED)
UURemoteS 3268   4b   63u  IPv4 0x2      0t0  UDP 192.168.1.5:5000->203.0.113.9:6000
UURemoteS 3268   4b   64u  IPv4 0x3      0t0  TCP *:7000 (LISTEN)
UURemoteS 3268   4b   65u  IPv4 0x4      0t0  TCP 127.0.0.1:1->127.0.0.1:2 (ESTABLISHED)
UURemoteS 3268   4b   66u  IPv4 0x5      0t0  UDP *:5353
`
	if n := countConnLines(out); n != 2 {
		t.Fatalf("want 2, got %d", n)
	}
}
