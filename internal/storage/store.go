package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"stageguard/internal/domain"
	"sync"
	"time"
)

type Store struct {
	mu           sync.RWMutex
	path         string
	data         domain.Snapshot
	snapshotFile *os.File
}

func Open(path string) (*Store, error) {
	if path == "" {
		path = "stageguard-data.json"
	}
	s := &Store{path: path, data: domain.NewSnapshot()}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取数据文件: %w", err)
	}
	if err := json.Unmarshal(b, &s.data); err != nil {
		return nil, fmt.Errorf("解析数据文件: %w", err)
	}
	normalize(&s.data)
	if s.data.SchemaVersion != 1 {
		return nil, fmt.Errorf("不支持的schemaVersion: %d", s.data.SchemaVersion)
	}
	want := s.data.IntegrityHash
	if want == "" || want != domain.HashPersistedSnapshot(s.data) {
		return nil, errors.New("快照完整性校验失败")
	}
	if err := verifyEvents(s.data.Events); err != nil {
		return nil, err
	}
	if err := verifyLedger(path+".events.jsonl", s.data.Events); err != nil {
		return nil, err
	}
	return s, nil
}
func normalize(s *domain.Snapshot) {
	if s.Dossiers == nil {
		s.Dossiers = map[string]domain.SafetyDossier{}
	}
	if s.Inspections == nil {
		s.Inspections = map[string]domain.InspectionItem{}
	}
	if s.Issues == nil {
		s.Issues = map[string]domain.SafetyIssue{}
	}
	if s.Permits == nil {
		s.Permits = map[string]domain.ActivationPermit{}
	}
	if s.Events == nil {
		s.Events = []domain.AuditEvent{}
	}
	if s.BatchReceipts == nil {
		s.BatchReceipts = map[string]domain.BatchReceipt{}
	}
}
func (s *Store) Path() string              { return s.path }
func (s *Store) Snapshot() domain.Snapshot { s.mu.RLock(); defer s.mu.RUnlock(); return clone(s.data) }
func (s *Store) CreateDossier(d domain.SafetyDossier, actor string) (domain.SafetyDossier, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.data.Dossiers[d.ID]; ok {
		return domain.SafetyDossier{}, errors.New("档案编号已存在")
	}
	w := clone(s.data)
	w.Dossiers[d.ID] = d
	e := event(d.ID, "dossier.created", actor, d.Version, "创建演出场次安全档案", w.Events)
	w.Events = append(w.Events, e)
	if err := s.persist(w); err != nil {
		return domain.SafetyDossier{}, err
	}
	s.data = w
	return d, nil
}
func (s *Store) AddInspection(i domain.InspectionItem, actor string, expected int) (domain.InspectionItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.data.Dossiers[i.DossierID]
	if !ok {
		return domain.InspectionItem{}, errors.New("档案不存在")
	}
	if expected >= 0 && d.Version != expected {
		return domain.InspectionItem{}, fmt.Errorf("版本冲突: 当前版本%d", d.Version)
	}
	if old, ok := s.data.Inspections[i.ID]; ok {
		return old, nil
	}
	w := clone(s.data)
	w.Inspections[i.ID] = i
	if d.Status == domain.StatusDraft {
		if err := d.Transition(domain.StatusInspecting); err != nil {
			return domain.InspectionItem{}, err
		}
		d.Status = domain.StatusInspecting
	}
	d.Version++
	d.UpdatedAt = time.Now().UTC()
	w.Dossiers[i.DossierID] = d
	e := event(d.ID, "inspection.recorded", actor, d.Version, i.CheckCode, w.Events)
	w.Events = append(w.Events, e)
	if err := s.persist(w); err != nil {
		return domain.InspectionItem{}, err
	}
	s.data = w
	return i, nil
}
func (s *Store) Mutate(expected int, dossierID, actor, kind, detail string, fn func(*domain.Snapshot) error) (domain.SafetyDossier, domain.AuditEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.data.Dossiers[dossierID]
	if !ok {
		return domain.SafetyDossier{}, domain.AuditEvent{}, errors.New("档案不存在")
	}
	if expected >= 0 && d.Version != expected {
		return domain.SafetyDossier{}, domain.AuditEvent{}, fmt.Errorf("版本冲突: 当前版本%d", d.Version)
	}
	w := clone(s.data)
	if err := fn(&w); err != nil {
		return domain.SafetyDossier{}, domain.AuditEvent{}, err
	}
	d = w.Dossiers[dossierID]
	d.Version++
	d.UpdatedAt = time.Now().UTC()
	w.Dossiers[dossierID] = d
	e := event(dossierID, kind, actor, d.Version, detail, w.Events)
	w.Events = append(w.Events, e)
	if err := s.persist(w); err != nil {
		return domain.SafetyDossier{}, domain.AuditEvent{}, err
	}
	s.data = w
	return d, e, nil
}
func (s *Store) persist(data domain.Snapshot) error {
	normalize(&data)
	data.IntegrityHash = domain.HashPersistedSnapshot(data)
	if dir := filepath.Dir(s.path); dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	ledger := s.path + ".events.jsonl"
	ledgerTmp := ledger + ".tmp"
	var lines []byte
	for _, event := range data.Events {
		line, marshalErr := json.Marshal(event)
		if marshalErr != nil {
			return marshalErr
		}
		lines = append(lines, line...)
		lines = append(lines, '\n')
	}
	if err = os.WriteFile(ledgerTmp, lines, 0644); err != nil {
		return err
	}
	if err = os.Rename(ledgerTmp, ledger); err != nil {
		return err
	}
	if s.snapshotFile == nil {
		s.snapshotFile, err = os.OpenFile(s.path, os.O_CREATE|os.O_RDWR, 0644)
		if err != nil {
			return err
		}
	}
	if err = s.snapshotFile.Truncate(0); err != nil {
		return err
	}
	if _, err = s.snapshotFile.Seek(0, 0); err != nil {
		return err
	}
	if _, err = s.snapshotFile.Write(b); err != nil {
		return err
	}
	return s.snapshotFile.Sync()
}
func event(dossierID, kind, actor string, version int, detail string, events []domain.AuditEvent) domain.AuditEvent {
	e := domain.AuditEvent{ID: fmt.Sprintf("evt-%d", time.Now().UnixNano()), DossierID: dossierID, Type: kind, Actor: actor, At: time.Now().UTC(), Version: version, Detail: detail}
	if len(events) > 0 {
		e.PrevHash = events[len(events)-1].Hash
	}
	e.Hash = domain.HashEvent(e)
	return e
}
func clone(in domain.Snapshot) domain.Snapshot {
	b, _ := json.Marshal(in)
	var out domain.Snapshot
	_ = json.Unmarshal(b, &out)
	normalize(&out)
	return out
}
