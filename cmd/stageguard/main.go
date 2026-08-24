package main

import (
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"stageguard/internal/domain"
	"stageguard/internal/httpapi"
	"stageguard/internal/storage"
	"stageguard/internal/workflow"
	"strconv"
	"syscall"
	"time"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:19081", "监听地址")
	selfcheck := flag.Bool("selfcheck", false, "运行有界全链路自检")
	data := flag.String("data", "stageguard-data.json", "本地账本路径")
	flag.Parse()
	resolved, err := resolveAddr(*addr)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if *selfcheck {
		if err := runSelfcheck(); err != nil {
			fmt.Fprintln(os.Stderr, "自检失败:", err)
			os.Exit(1)
		}
		fmt.Println("舞台机械安全核验全链路自检通过")
		return
	}
	store, err := storage.Open(*data)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	svc := workflow.New(store)
	server := &http.Server{Addr: resolved, Handler: httpapi.New(svc).Handler()}
	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}()
	fmt.Printf("舞台机械安全启用核验台监听 %s\n", resolved)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	_ = server.Close()
}
func resolveAddr(raw string) (string, error) {
	if raw == "" {
		raw = "127.0.0.1:19081"
	}
	if port := os.Getenv("PORT"); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return "", errors.New("PORT 必须是有效端口号")
		}
		raw = "127.0.0.1:" + port
	}
	host, _, err := net.SplitHostPort(raw)
	if err != nil {
		return "", fmt.Errorf("监听地址无效: %w", err)
	}
	if host != "127.0.0.1" && host != "localhost" && host != "::1" {
		return "", errors.New("服务只允许绑定回环地址")
	}
	return raw, nil
}
func runSelfcheck() error {
	dir, err := os.MkdirTemp("", "stageguard-selfcheck-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	store, err := storage.Open(filepath.Join(dir, "ledger.json"))
	if err != nil {
		return err
	}
	svc := workflow.New(store)
	d, err := svc.CreateDossier(workflow.CreateDossierCommand{ShowName: "自检演出", Venue: "一号剧场", CreatedBy: "工程师", ScheduledAt: time.Now().UTC().Add(time.Hour), EquipmentBoundary: []domain.Equipment{{ID: "hoist-01", Name: "主升降台", RatedLoadKg: 1000, IsolationBoundary: "东侧隔离线"}}})
	if err != nil {
		return err
	}
	_, err = svc.RecordInspectionBatch(workflow.BatchInspectionCommand{DossierID: d.ID, Actor: "工程师", ExpectedVersion: d.Version, Items: []workflow.InspectionCommand{
		{EquipmentID: "hoist-01", CheckCode: "LOAD_LIMIT", ObservedValue: "1200kg", MeasuredLoadKg: 1200, Result: "通过", Inspector: "工程师"},
		{EquipmentID: "hoist-01", CheckCode: "LIMIT_RESPONSE", LimitResponseMs: 220, Result: "通过", Inspector: "工程师"},
		{EquipmentID: "hoist-01", CheckCode: "EMERGENCY_STOP", EmergencyStopResult: "通过", Result: "通过", Inspector: "工程师"},
	}})
	if err != nil {
		return err
	}
	d = svc.Snapshot().Dossiers[d.ID]
	issues, err := svc.DetectIssues(d.ID, "工程师", d.Version)
	if err != nil || len(issues) == 0 {
		return errors.New("规则检测未生成问题")
	}
	d = svc.Snapshot().Dossiers[d.ID]
	now := time.Now().UTC()
	evidence := []domain.Evidence{{Name: "整改照片", Type: domain.EvidencePhoto, CollectedAt: now, Reference: "photo://selfcheck", Digest: "整改后现场"}, {Name: "复测记录", Type: domain.EvidenceRetest, CollectedAt: now, Reference: "retest://selfcheck", Digest: "载荷980kg"}}
	_, err = svc.SubmitRemediation(workflow.RemediationCommand{IssueID: issues[0].ID, Remediation: "调整限位并降低载荷", RetestData: "载荷980kg，急停通过，响应220ms", Evidence: evidence, Actor: "工程师", ExpectedVersion: d.Version})
	if err != nil {
		return err
	}
	d = svc.Snapshot().Dossiers[d.ID]
	_, err = svc.ReviewIssue(issues[0].ID, "安全员", string(domain.ReviewPassed), "自检复核通过", d.Version)
	if err != nil {
		return err
	}
	d = svc.Snapshot().Dossiers[d.ID]
	d, err = svc.Freeze(d.ID, "安全员", d.Version)
	if err != nil {
		return err
	}
	if d.Status != domain.StatusFrozen {
		return errors.New("档案未冻结")
	}
	_, err = svc.IssuePermit(d.ID, "值班经理", d.Version)
	if err != nil {
		return err
	}
	if svc.Snapshot().Dossiers[d.ID].Status != domain.StatusIssued {
		return errors.New("许可签发未推进状态")
	}
	return nil
}
