package domain

import "time"

type DossierStatus string

const (
	StatusDraft       DossierStatus = "草稿"
	StatusInspecting  DossierStatus = "检查中"
	StatusRemediation DossierStatus = "待整改"
	StatusReview      DossierStatus = "待复核"
	StatusFrozen      DossierStatus = "已冻结"
	StatusIssued      DossierStatus = "已签发"
)

type IssueSeverity string

const (
	SeverityLow      IssueSeverity = "低"
	SeverityMedium   IssueSeverity = "中"
	SeverityHigh     IssueSeverity = "高"
	SeverityCritical IssueSeverity = "严重"
)

type ReviewDecision string

const (
	ReviewPending  ReviewDecision = "待处理"
	ReviewPassed   ReviewDecision = "通过"
	ReviewReturned ReviewDecision = "退回"
)

type Equipment struct {
	ID                string  `json:"id"`
	Name              string  `json:"name"`
	RatedLoadKg       float64 `json:"ratedLoadKg"`
	IsolationBoundary string  `json:"isolationBoundary"`
}
type SafetyDossier struct {
	ID                string            `json:"id"`
	ShowName          string            `json:"showName"`
	Venue             string            `json:"venue"`
	ScheduledAt       time.Time         `json:"scheduledAt"`
	EquipmentBoundary []Equipment       `json:"equipmentBoundary"`
	Status            DossierStatus     `json:"status"`
	Version           int               `json:"version"`
	CreatedBy         string            `json:"createdBy"`
	UpdatedAt         time.Time         `json:"updatedAt"`
	FrozenHash        string            `json:"frozenHash,omitempty"`
	LastDetectedAt    *time.Time        `json:"lastDetectedAt,omitempty"`
	Revisions         []DossierRevision `json:"revisions,omitempty"`
}
type DossierRevision struct {
	Version       int       `json:"version"`
	Actor         string    `json:"actor"`
	At            time.Time `json:"at"`
	BeforeSummary string    `json:"beforeSummary"`
	AfterSummary  string    `json:"afterSummary"`
}
type InspectionItem struct {
	ID                  string    `json:"id"`
	DossierID           string    `json:"dossierID"`
	EquipmentID         string    `json:"equipmentID"`
	CheckCode           string    `json:"checkCode"`
	ObservedValue       string    `json:"observedValue"`
	MeasuredLoadKg      float64   `json:"measuredLoadKg,omitempty"`
	LimitResponseMs     int       `json:"limitResponseMs,omitempty"`
	EmergencyStopResult string    `json:"emergencyStopResult,omitempty"`
	Result              string    `json:"result"`
	Inspector           string    `json:"inspector"`
	Notes               string    `json:"notes"`
	RecordedAt          time.Time `json:"recordedAt"`
}
type RemediationRevision struct {
	Revision     int            `json:"revision"`
	Remediation  string         `json:"remediation"`
	RetestData   string         `json:"retestData"`
	EvidenceRefs []string       `json:"evidenceRefs"`
	Evidence     []Evidence     `json:"evidence,omitempty"`
	SubmittedBy  string         `json:"submittedBy"`
	SubmittedAt  time.Time      `json:"submittedAt"`
	Decision     ReviewDecision `json:"decision"`
	ReviewNote   string         `json:"reviewNote,omitempty"`
	ReviewedBy   string         `json:"reviewedBy,omitempty"`
	ReviewedAt   *time.Time     `json:"reviewedAt,omitempty"`
}
type Evidence struct {
	Name        string    `json:"name"`
	Type        string    `json:"type"`
	CollectedAt time.Time `json:"collectedAt"`
	Reference   string    `json:"reference"`
	Digest      string    `json:"digest"`
}

const (
	EvidencePhoto    = "现场照片"
	EvidenceRetest   = "复测记录"
	EvidenceDocument = "文档"
	EvidenceVideo    = "现场视频"
)

type SafetyIssue struct {
	ID                 string                `json:"id"`
	DossierID          string                `json:"dossierID"`
	InspectionItemID   string                `json:"inspectionItemID"`
	Severity           IssueSeverity         `json:"severity"`
	RuleCode           string                `json:"ruleCode"`
	Description        string                `json:"description"`
	Remediation        string                `json:"remediation"`
	RetestData         string                `json:"retestData"`
	EvidenceRefs       []string              `json:"evidenceRefs"`
	Evidence           []Evidence            `json:"evidence,omitempty"`
	ReviewDecision     ReviewDecision        `json:"reviewDecision"`
	ReviewNote         string                `json:"reviewNote,omitempty"`
	ResolvedAt         *time.Time            `json:"resolvedAt,omitempty"`
	Revision           int                   `json:"revision"`
	UpdatedAt          time.Time             `json:"updatedAt"`
	Revisions          []RemediationRevision `json:"revisions"`
	StableKey          string                `json:"stableKey"`
	EquipmentID        string                `json:"equipmentID"`
	CheckCode          string                `json:"checkCode"`
	LastDetectedAt     *time.Time            `json:"lastDetectedAt,omitempty"`
	PendingElimination bool                  `json:"pendingElimination"`
	ReopenCount        int                   `json:"reopenCount"`
	ReviewedRevision   int                   `json:"reviewedRevision,omitempty"`
	ReviewedBy         string                `json:"reviewedBy,omitempty"`
	ReviewedAt         *time.Time            `json:"reviewedAt,omitempty"`
}
type ActivationPermit struct {
	ID            string    `json:"id"`
	DossierID     string    `json:"dossierID"`
	FrozenVersion int       `json:"frozenVersion"`
	IssuedBy      string    `json:"issuedBy"`
	IssuedAt      time.Time `json:"issuedAt"`
	ContentHash   string    `json:"contentHash"`
	PermitCode    string    `json:"permitCode"`
	AuditEventIDs []string  `json:"auditEventIDs"`
}
type AuditEvent struct {
	ID        string    `json:"id"`
	DossierID string    `json:"dossierID"`
	Type      string    `json:"type"`
	Actor     string    `json:"actor"`
	At        time.Time `json:"at"`
	Version   int       `json:"version"`
	Detail    string    `json:"detail"`
	PrevHash  string    `json:"prevHash"`
	Hash      string    `json:"hash"`
}
type Snapshot struct {
	SchemaVersion   int                         `json:"schemaVersion"`
	IntegrityHash   string                      `json:"integrityHash"`
	Dossiers        map[string]SafetyDossier    `json:"dossiers"`
	Inspections     map[string]InspectionItem   `json:"inspections"`
	Issues          map[string]SafetyIssue      `json:"issues"`
	Permits         map[string]ActivationPermit `json:"permits"`
	Events          []AuditEvent                `json:"events"`
	BatchReceipts   map[string]BatchReceipt     `json:"batchReceipts,omitempty"`
	DossierReceipts map[string]DossierReceipt   `json:"dossierReceipts,omitempty"`
}

type DossierReceipt struct {
	DossierID   string `json:"dossierID"`
	Fingerprint string `json:"fingerprint"`
}

type BatchReceipt struct {
	DossierID     string   `json:"dossierID"`
	Fingerprint   string   `json:"fingerprint"`
	Version       int      `json:"version"`
	Added         int      `json:"added"`
	Corrected     int      `json:"corrected"`
	InspectionIDs []string `json:"inspectionIDs"`
}

func NewSnapshot() Snapshot {
	return Snapshot{SchemaVersion: 1, Dossiers: map[string]SafetyDossier{}, Inspections: map[string]InspectionItem{}, Issues: map[string]SafetyIssue{}, Permits: map[string]ActivationPermit{}, Events: []AuditEvent{}, BatchReceipts: map[string]BatchReceipt{}, DossierReceipts: map[string]DossierReceipt{}}
}
