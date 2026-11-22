package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"
)

var (
	addr    = flag.String("addr", "http://localhost:8080", "Karl API address")
	rawJSON = flag.Bool("json", false, "Output raw JSON")
)

func main() {
	flag.Parse()
	args := flag.Args()
	if len(args) == 0 {
		usage()
		os.Exit(1)
	}

	var err error
	switch args[0] {
	case "ping":
		err = doGet("/health")
	case "stats":
		err = doGet("/api/v1/stats")
	case "sessions":
		err = doGet("/api/v1/sessions")
	case "session":
		if len(args) < 2 {
			fatal("usage: karl-ctl session <id>")
		}
		err = doGet("/api/v1/sessions/" + args[1])
	case "delete":
		if len(args) < 2 {
			fatal("usage: karl-ctl delete <session-id>")
		}
		err = doReq("DELETE", "/api/v1/sessions/"+args[1], nil)
	case "recording":
		if len(args) < 3 {
			fatal("usage: karl-ctl recording start|stop <session-id>")
		}
		action := args[1]
		if action != "start" && action != "stop" {
			fatal("recording action must be 'start' or 'stop'")
		}
		body := map[string]string{"session_id": args[2]}
		b, _ := json.Marshal(body)
		err = doReq("POST", "/api/v1/recording/"+action, strings.NewReader(string(b)))
	case "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", args[0])
		usage()
		os.Exit(1)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func doGet(path string) error {
	return doReq("GET", path, nil)
}

func doReq(method, path string, body io.Reader) error {
	req, err := http.NewRequest(method, *addr+path, body)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(data))
	}
	if *rawJSON {
		fmt.Println(string(data))
		return nil
	}
	return prettyPrint(data)
}

func prettyPrint(data []byte) error {
	var arr []map[string]any
	if json.Unmarshal(data, &arr) == nil && len(arr) > 0 {
		printTable(arr)
		return nil
	}
	var obj map[string]any
	if json.Unmarshal(data, &obj) == nil {
		printKV(obj, "")
		return nil
	}
	fmt.Println(string(data))
	return nil
}

func printTable(rows []map[string]any) {
	if len(rows) == 0 {
		fmt.Println("(empty)")
		return
	}
	var keys []string
	for k := range rows[0] {
		keys = append(keys, k)
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, strings.Join(keys, "\t"))
	fmt.Fprintln(w, strings.Repeat("---\t", len(keys)))
	for _, row := range rows {
		vals := make([]string, len(keys))
		for i, k := range keys {
			vals[i] = fmt.Sprintf("%v", row[k])
		}
		fmt.Fprintln(w, strings.Join(vals, "\t"))
	}
	w.Flush()
}

func printKV(obj map[string]any, indent string) {
	for k, v := range obj {
		switch vt := v.(type) {
		case map[string]any:
			fmt.Printf("%s%s:\n", indent, k)
			printKV(vt, indent+"  ")
		default:
			fmt.Printf("%s%-20s %v\n", indent, k+":", v)
		}
	}
}

func usage() {
	fmt.Print(`karl-ctl - Karl media engine CLI (rtpengine-ctl equivalent)

Usage: karl-ctl [flags] <command> [args]

Commands:
  ping                         Health check
  sessions                     List all sessions
  session <id>                 Show session details
  stats                        Show engine stats
  delete <session-id>          Delete a session
  recording start <session-id> Start recording
  recording stop <session-id>  Stop recording
  help                         Show this help

Flags:
  -addr string   API address (default "http://localhost:8080")
  -json          Output raw JSON
`)
}

func fatal(msg string) {
	fmt.Fprintln(os.Stderr, msg)
	os.Exit(1)
}
