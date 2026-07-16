# SaaS 전환을 위한 데이터베이스 마이그레이션 및 리팩토링 작업 계획서

본 문서는 단일 테넌트 파일 기반인 SQLite 구조의 `go-watchdog` 서비스를 다중 사용자(Multi-tenant)를 지원하는 고가용성 SaaS(Software as a Service) 서비스로 전환하기 위한 코드 리팩토링 및 데이터베이스 마이그레이션 단계별 계획을 정의합니다.

---

## 1. 아키텍처 개요 및 목표

### 1.1 현재 아키텍처 (Single-Tenant)
* **Application:** 단일 Go 백엔드 프로세스
* **Database:** 파일 기반 SQLite (`monitoring.db`)
* **한계:** 
  * 여러 백엔드 웹 서버 수평 확장 불가 (단일 파일 잠금 이슈)
  * 대규모 에이전트 연결 시 초당 쓰기 부하 분산 불가 (`SQLITE_BUSY` 발생)

### 1.2 대상 아키텍처 (Multi-Tenant SaaS)
* **Application:** 다중 백엔드 웹 서버 (AWS ECS/EKS 또는 Kubernetes로 로드밸런싱)
* **Configuration DB:** **PostgreSQL** (사용자 계정, 요금제, 테넌트 정보, 모니터링 대상 설정)
* **Metrics/Log DB:** **TimescaleDB** (수억 건에 달하는 대용량 시계열 성능 메트릭 및 헬스 체크 로그 저장/압축)
* **구조 변경:**
  ```mermaid
  graph TD
      Agent[watchdog-agent] -->|HTTPS Metrics| LB[Load Balancer]
      LB --> Server1[watchdog-server Instance 1]
      LB --> Server2[watchdog-server Instance 2]
      Server1 --> Postgres[(PostgreSQL / TimescaleDB)]
      Server2 --> Postgres
  ```

---

## 2. 1단계: 백엔드 코드 리팩토링 (Repository 패턴 도입)

현재 백엔드 코드(`handler.go`, `health_runner.go` 등)는 SQLite 의존적인 전역 함수나 SQL 구문을 직접 호출하고 있습니다. 이를 해결하기 위해 DB 세부 구현을 숨기는 **Repository Interface**를 정의합니다.

### 2.1 저장소 인터페이스 정의 (`server/store.go` 신설)
```go
package main

import (
	"hinet-watchdog/common"
)

// DataStore는 모든 데이터베이스 연산을 캡슐화하는 공통 인터페이스입니다.
type DataStore interface {
	// DB 생명주기 관리
	Close() error

	// 테넌트 및 설정 관리
	GetActiveTargets() ([]*HealthTarget, error)
	SaveHealthTarget(t *HealthTarget) error
	DeleteHealthTarget(id int64) error

	// 헬스 로그 및 메트릭 기록 (쓰기)
	SaveHealthLog(targetID int64, statusCode int, latencyMs int, isSuccess bool, errMsg string) error
	SaveMetric(m *common.Metric) error

	// 상태 대시보드 조회 (읽기)
	GetLatestMetrics() ([]*common.Metric, error)
	GetLatestTargetStatus(targetID int64) (string, error)
	GetHealthTargetsWithStatus() ([]*HealthTargetStatus, error)
}
```

### 2.2 SQLite 구현체 작성 (`server/sqlite_store.go` 신설)
기존 `server/db.go` 및 SQL 쿼리를 `SQLiteStore` 구조체 구현으로 이전합니다.
```go
package main

import (
	"database/sql"
	"hinet-watchdog/common"
)

type SQLiteStore struct {
	db *sql.DB
}

func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	// 기존 InitDB 로직 수행
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	// 커넥션 풀 설정 및 인덱스 생성
	return &SQLiteStore{db: db}, nil
}

func (s *SQLiteStore) SaveMetric(m *common.Metric) error {
	// 기존 SaveMetric SQL 쿼리 실행
	return nil
}
// ... 나머지 DataStore 인터페이스 메서드 구현
```

### 2.3 의존성 주입 (Dependency Injection) 적용
`main.go` 및 서버 핸들러 구조체에 인터페이스 타입의 멤버 변수를 할당합니다.
```go
// server/handler.go 변경 예시
type Server struct {
	store DataStore // sql.DB 대신 추상화된 DataStore를 참조
}
```

### 2.4 config.json 분기 설정 및 드라이버 팩토리 패턴 구현
동일한 백엔드 바이너리를 환경에 맞게 동적으로 로드할 수 있도록 설정을 다변화합니다.

#### A. config.json 설정 명세 추가
개발 환경(`sqlite`)과 상용 서비스 환경(`postgres`)을 구분하기 위한 데이터베이스 연결 유형 설정을 확장합니다.
```json
{
  "port": 9090,
  "auth_token": "watchdog-secret-token",
  
  "db_type": "postgres", 
  "db_path": "monitoring.db", 
  "db_dsn": "postgres://username:password@localhost:5432/monitoring?sslmode=disable",
  
  "retention_days": 7
}
```

#### B. 드라이버 팩토리 로직 구현 (`server/main.go` 부트스트랩)
애플리케이션 시작 단계에서 설정 파일의 `db_type`을 파싱하여 동적으로 `DataStore` 구현체를 인스턴스화합니다.
```go
func main() {
	cfg := LoadConfig("config.json")

	var store DataStore
	var err error

	// 설정값에 따라 데이터베이스 드라이버 동적 선택 (Factory Pattern)
	switch cfg.DBType {
	case "postgres":
		log.Println("[Server] Connecting to PostgreSQL database...")
		store, err = NewPostgresStore(cfg.DBDSN)
	case "sqlite":
		fallthrough
	default:
		log.Println("[Server] Connecting to SQLite database...")
		store, err = NewSQLiteStore(cfg.DBPath)
	}

	if err != nil {
		log.Fatalf("[Server] [Fatal] Failed to initialize database store: %v", err)
	}
	defer store.Close()

	// 이후 로직(Health Check Runner, HTTP API Handlers)은 
	// 실제 구동 DB 종류를 인지할 필요 없이 store 변수를 통해 인터페이스 메서드만 실행합니다.
}
```

---

## 3. 2단계: PostgreSQL & TimescaleDB 스키마 설계

SaaS 전환 시, 테넌트(Tenant/고객사)별 데이터 격리를 위한 `tenant_id` 필드가 필수적으로 추가되어야 합니다.

### 3.1 메타데이터 테이블 (PostgreSQL)
```sql
-- 1. 테넌트(고객사) 테이블
CREATE TABLE tenants (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    plan_tier VARCHAR(20) DEFAULT 'FREE', -- 요금제 정보
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- 2. 사용자 계정 테이블
CREATE TABLE users (
    id SERIAL PRIMARY KEY,
    tenant_id INTEGER REFERENCES tenants(id) ON DELETE CASCADE,
    email VARCHAR(255) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    role VARCHAR(20) DEFAULT 'USER',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- 3. 모니터링 대상 타겟 설정 테이블
CREATE TABLE health_targets (
    id BIGSERIAL PRIMARY KEY,
    tenant_id INTEGER REFERENCES tenants(id) ON DELETE CASCADE,
    name VARCHAR(100) NOT NULL,
    url TEXT NOT NULL,
    interval_seconds INTEGER DEFAULT 30,
    timeout_seconds INTEGER DEFAULT 5,
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
```

### 3.2 시계열 로그 및 성능 메트릭 테이블 (TimescaleDB)
시계열 데이터는 데이터 보존정책 및 파티셔닝(Hypertables)이 지원되는 TimescaleDB 형태로 구축합니다.
```sql
-- 4. 헬스 체크 로그 테이블
CREATE TABLE health_logs (
    id BIGSERIAL,
    target_id BIGINT NOT NULL,
    status_code INTEGER,
    latency_ms INTEGER,
    is_success BOOLEAN NOT NULL,
    error_message TEXT,
    timestamp TIMESTAMP WITH TIME ZONE NOT NULL
);
-- 헬스 로그를 시계열 하이퍼테이블로 변환 (7일 단위 파티셔닝)
SELECT create_hypertable('health_logs', 'timestamp', chunk_time_interval => INTERVAL '7 days');
CREATE INDEX idx_health_logs_target_time ON health_logs(target_id, timestamp DESC);

-- 5. 시스템 성능 메트릭 테이블
CREATE TABLE metrics (
    id BIGSERIAL,
    agent_id VARCHAR(100) NOT NULL,
    cpu_percent DOUBLE PRECISION,
    mem_total_gb DOUBLE PRECISION,
    mem_used_gb DOUBLE PRECISION,
    mem_percent DOUBLE PRECISION,
    timestamp TIMESTAMP WITH TIME ZONE NOT NULL
);
SELECT create_hypertable('metrics', 'timestamp', chunk_time_interval => INTERVAL '7 days');
CREATE INDEX idx_metrics_agent_time ON metrics(agent_id, timestamp DESC);
```

---

## 4. 3단계: PostgresStore 구현 (`server/postgres_store.go` 신설)

Postgres 전용 드라이버 라이브러리인 `pgx` 또는 Go 표준 `database/sql`을 통해 `PostgresStore`를 구현합니다.

```go
package main

import (
	"database/sql"
	_ "github.com/jackc/pgx/v5/stdlib" // pgx 드라이버 사용
	"hinet-watchdog/common"
)

type PostgresStore struct {
	db *sql.DB
}

func NewPostgresStore(dsn string) (*PostgresStore, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	
	// SaaS용 대규모 세션 풀 설정
	db.SetMaxOpenConns(50)
	db.SetMaxIdleConns(25)
	
	return &PostgresStore{db: db}, nil
}

// DataStore 인터페이스의 Postgres 구현체들 작성...
```

---

## 5. 4단계: 무중단 마이그레이션 시나리오

서비스 중단 없이 SQLite에서 PostgreSQL로 데이터를 이전하고 전환하는 절차입니다.

```mermaid
sequenceDiagram
    participant Config as 로컬 SQLite
    participant Tool as Migration Script
    participant PG as PostgreSQL Cloud
    
    Note over Tool: 1. DB 스키마 생성
    Tool->>PG: CREATE TABLE 및 하이퍼테이블 생성
    Note over Tool: 2. 과거 로그 데이터 이전
    Tool->>Config: SELECT * FROM health_logs/metrics
    Config-->>Tool: 데이터 스트리밍
    Tool->>PG: COPY (Bulk Insert) 데이터 전송
    Note over Tool: 3. 신규 배포 및 무중단 전환
```

1. **스키마 배포:** 클라우드 PostgreSQL/TimescaleDB 인스턴스에 위 3장의 SQL 스키마를 배포합니다.
2. **이전용 일회성 마이그레이션 툴 개발:** SQLite 데이터를 읽어 Postgres에 Bulk Insert(`COPY` 프로토콜 등) 해주는 가벼운 Go/Python 스크립트를 작성하여 구동합니다.
3. **듀얼 라이팅(Optional):** 데이터 유실이 절대로 허용되지 않는 경우, 코드상에서 `MultiStore`를 구현하여 SQLite와 Postgres에 동시에 쓰기 작업을 수행하게 한 뒤 최종적으로 읽기 대상을 이전합니다. (일반 모니터링 툴은 단순 DB 교체 및 서버 재부팅 방식으로 충분합니다.)
4. **설정 업데이트:** `config.json` 설정을 파일 경로 대신 PostgreSQL DSN 연결값으로 수정하여 서버를 배포합니다.
