package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

const dockerSocket = "/var/run/docker.sock"

type request struct {
	Mode             string `json:"mode"`
	ApplicationID    string `json:"application_id"`
	InteractionToken string `json:"interaction_token"`
}

type dockerInspect struct {
	State struct {
		Running bool `json:"Running"`
		Health  *struct {
			Status string `json:"Status"`
		} `json:"Health"`
	} `json:"State"`
	Config struct {
		Labels map[string]string `json:"Labels"`
	} `json:"Config"`
}

type dockerContainer struct {
	ID     string            `json:"Id"`
	Labels map[string]string `json:"Labels"`
}

var (
	busyMu sync.Mutex
	busy   bool

	httpClient = &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "unix", dockerSocket)
			},
		},
	}
)

func main() {
	http.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	http.HandleFunc("/restart", handleRestart)

	addr := ":8090"
	log.Printf("restart-controller listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}

func handleRestart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	secret := strings.TrimSpace(os.Getenv("RESTART_CONTROLLER_TOKEN"))
	if secret == "" || r.Header.Get("Authorization") != "Bearer "+secret {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req request
	if err := json.NewDecoder(io.LimitReader(r.Body, 8192)).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if req.Mode != "bot" && req.Mode != "all" {
		http.Error(w, "invalid mode", http.StatusBadRequest)
		return
	}

	busyMu.Lock()
	if busy {
		busyMu.Unlock()
		http.Error(w, "restart already in progress", http.StatusConflict)
		return
	}
	busy = true
	busyMu.Unlock()

	w.WriteHeader(http.StatusAccepted)

	go func() {
		defer func() {
			busyMu.Lock()
			busy = false
			busyMu.Unlock()
		}()

		err := performRestart(req.Mode)
		if err != nil {
			log.Printf("restart failed: %v", err)
			notify(req, "❌ 再起動に失敗しました: "+err.Error())
			return
		}

		if req.Mode == "all" {
			notify(req, "✅ AutoMuteUs / Galactus / Redis / PostgreSQL の再起動が完了しました。")
		} else {
			notify(req, "✅ AutoMuteUsの再起動が完了しました。")
		}
	}()
}

func performRestart(mode string) error {
	project, err := composeProject()
	if err != nil {
		return err
	}

	required := []string{"automuteus"}
	if mode == "all" {
		required = []string{"automuteus", "galactus", "redis", "postgres"}
	}

	services := make(map[string]string, len(required))
	for _, name := range required {
		id, err := serviceContainer(project, name)
		if err != nil {
			return err
		}
		services[name] = id
	}

	if mode == "bot" {
		if err := dockerPost("/containers/" + services["automuteus"] + "/restart?t=20"); err != nil {
			return fmt.Errorf("restart automuteus: %w", err)
		}
		if err := waitRunning(services["automuteus"], 90*time.Second); err != nil {
			return fmt.Errorf("automuteus: %w", err)
		}
		if err := waitAutoMuteUsReady(120 * time.Second); err != nil {
			return fmt.Errorf("automuteus: %w", err)
		}
		return nil
	}

	if err := dockerPost("/containers/" + services["automuteus"] + "/stop?t=20"); err != nil {
		return fmt.Errorf("stop automuteus: %w", err)
	}
	if err := dockerPost("/containers/" + services["galactus"] + "/stop?t=15"); err != nil {
		return fmt.Errorf("stop galactus: %w", err)
	}
	if err := dockerPost("/containers/" + services["redis"] + "/restart?t=10"); err != nil {
		return fmt.Errorf("restart redis: %w", err)
	}
	if err := dockerPost("/containers/" + services["postgres"] + "/restart?t=20"); err != nil {
		return fmt.Errorf("restart postgres: %w", err)
	}
	if err := waitHealthy(services["redis"], 120*time.Second); err != nil {
		return fmt.Errorf("redis: %w", err)
	}
	if err := waitHealthy(services["postgres"], 120*time.Second); err != nil {
		return fmt.Errorf("postgres: %w", err)
	}
	if err := dockerPost("/containers/" + services["galactus"] + "/start"); err != nil {
		return fmt.Errorf("start galactus: %w", err)
	}
	if err := waitRunning(services["galactus"], 60*time.Second); err != nil {
		return fmt.Errorf("galactus: %w", err)
	}

	time.Sleep(3 * time.Second)

	if err := dockerPost("/containers/" + services["automuteus"] + "/start"); err != nil {
		return fmt.Errorf("start automuteus: %w", err)
	}
	if err := waitRunning(services["automuteus"], 90*time.Second); err != nil {
		return fmt.Errorf("automuteus: %w", err)
	}
	if err := waitAutoMuteUsReady(120 * time.Second); err != nil {
		return fmt.Errorf("automuteus: %w", err)
	}
	return nil
}

func composeProject() (string, error) {
	host, err := os.Hostname()
	if err != nil {
		return "", err
	}

	var in dockerInspect
	if err := dockerGetJSON("/containers/"+url.PathEscape(host)+"/json", &in); err != nil {
		return "", fmt.Errorf("inspect controller: %w", err)
	}

	project := in.Config.Labels["com.docker.compose.project"]
	if project == "" {
		return "", fmt.Errorf("compose project label not found")
	}
	return project, nil
}

func serviceContainer(project, service string) (string, error) {
	filters, err := json.Marshal(map[string][]string{
		"label": {
			"com.docker.compose.project=" + project,
			"com.docker.compose.service=" + service,
		},
	})
	if err != nil {
		return "", err
	}

	var list []dockerContainer
	path := "/containers/json?all=1&filters=" + url.QueryEscape(string(filters))
	if err := dockerGetJSON(path, &list); err != nil {
		return "", err
	}
	if len(list) != 1 {
		return "", fmt.Errorf("service %s: expected 1 container, got %d", service, len(list))
	}
	return list[0].ID, nil
}

func waitHealthy(id string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var in dockerInspect
		if err := dockerGetJSON("/containers/"+id+"/json", &in); err == nil &&
			in.State.Running &&
			in.State.Health != nil &&
			in.State.Health.Status == "healthy" {
			return nil
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("healthcheck timeout")
}

func waitRunning(id string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var in dockerInspect
		if err := dockerGetJSON("/containers/"+id+"/json", &in); err == nil && in.State.Running {
			return nil
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("running timeout")
}

func waitAutoMuteUsReady(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 5 * time.Second}

	for time.Now().Before(deadline) {
		resp, err := client.Get("http://automuteus:8080/ready")
		if err == nil {
			body, readErr := io.ReadAll(io.LimitReader(resp.Body, 4096))
			resp.Body.Close()
			if readErr == nil &&
				resp.StatusCode == http.StatusOK &&
				strings.HasPrefix(string(body), "ready") {
				return nil
			}
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("readiness timeout")
}

func dockerGetJSON(path string, out any) error {
	req, err := http.NewRequest(http.MethodGet, "http://docker"+path, nil)
	if err != nil {
		return err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("docker GET %s: %s %s", path, resp.Status, strings.TrimSpace(string(body)))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func dockerPost(path string) error {
	req, err := http.NewRequest(http.MethodPost, "http://docker"+path, nil)
	if err != nil {
		return err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 && resp.StatusCode != http.StatusNotModified {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("docker POST %s: %s %s", path, resp.Status, strings.TrimSpace(string(body)))
	}
	return nil
}

func notify(req request, content string) {
	if req.ApplicationID == "" || req.InteractionToken == "" {
		return
	}

	body, err := json.Marshal(map[string]any{
		"content": content,
		"flags":   64,
	})
	if err != nil {
		log.Printf("discord followup marshal failed: %v", err)
		return
	}

	endpoint := "https://discord.com/api/v10/webhooks/" +
		url.PathEscape(req.ApplicationID) + "/" +
		url.PathEscape(req.InteractionToken)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(endpoint, "application/json", bytes.NewReader(body))
	if err != nil {
		log.Printf("discord followup failed: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		log.Printf("discord followup returned %s", resp.Status)
	}
}
