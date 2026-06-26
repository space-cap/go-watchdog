# [구현 계획서] 경량형 서버 상태 감시 시스템 (go-watchdog) 구축

운영 중인 윈도우 서버의 성능 자원(CPU, Memory, Disk)을 주기적으로 수집하여 중심 백엔드 서버로 전송하고, 이를 실시간 Vanilla JS Polling 방식으로 관제하는 경량형 웹 대시보드를 구축합니다.

---

## User Review Required

> [!IMPORTANT]
> **외부 라이브러리 사용 및 네트워크 포트 확보**
> * **자원 수집 라이브러리:** 윈도우 서버 환경에서 CPU, 메모리, 디스크 정보를 안정적이고 가볍게 수집하기 위해 `github.com/shirou/gopsutil/v3` 라이브러리를 사용하고자 합니다.
> * **네트워크 방화벽:** 에이전트가 백엔드 서버(포트 `9090`)로 HTTP POST 요청을 보낼 수 있도록 해당 백엔드 서버의 인바운드 방화벽 포트 오픈이 필요합니다.
> * **보안 토큰 설정:** 환경 변수 또는 `config.json` 설정 파일에 보안을 위한 임의의 고유 API Key 값을 지정해야 합니다.

---

## Open Questions

> [!NOTE]
> * **윈도우 서비스 구동 방식:** 에이전트 배포의 편의성을 위해 초기에는 실행 인자(Arguments)를 통해 직접 동작하도록 개발하고, 추후 필요 시 외부 도구(예: NSSM)를 통해 서비스로 등록하는 편이 간편할 것입니다. 해당 방식에 동의하시나요?
> * **데이터 삭제 기한:** 기획서에 명시된 14일 보존을 기본값으로 하되, 설정 파일에서 변경할 수 있도록 구성할 예정입니다.

---

## Proposed Changes

프로젝트 전체 구조는 모노레포 형태이며, Go 모듈 루트는 `go-watchdog`로 정의합니다.

### 1. 공통 데이터 모델 및 의존성 구성

#### [NEW] [go.mod](file:///h:/lee/go-watchdog/go.mod)
* 모노레포 구성을 위한 Go 모듈 파일 정의 및 외부 의존성(`gopsutil`, `go-sqlite3`) 등록.

#### [NEW] [metric.go](file:///h:/lee/go-watchdog/common/metric.go)
* 에이전트가 수집하여 백엔드로 보낼 `Metric` 및 `DiskInfo` 데이터 구조체(DTO) 정의.
* CPU 사용률(%), 메모리 사용 정보(총량/사용중/사용률), 드라이브별 디스크 용량 정보(총량/사용중/사용률)를 포함.

---

### 2. 에이전트 (Agent) 구성

#### [NEW] [config.go](file:///h:/lee/go-watchdog/agent/config.go)
* 에이전트 설정 구조체 및 파일(`config.json`)을 읽어오는 로더 구현.
* 설정 항목: 백엔드 주소(`ServerURL`), 보안 토큰(`AuthToken`), 수집 주기(`IntervalSeconds`).

#### [NEW] [collector.go](file:///h:/lee/go-watchdog/agent/collector.go)
* `gopsutil` 패키지를 사용하여 윈도우 시스템의 실제 CPU, RAM, Disk 사용률을 측정하고 `common.Metric` 데이터로 가공하여 반환하는 로직 구현.

#### [NEW] [sender.go](file:///h:/lee/go-watchdog/agent/sender.go)
* 수집된 데이터를 JSON으로 직렬화하여 백엔드 REST API(`POST /api/metrics`)로 전송하는 HTTP 클라이언트 구현.
* 요청 헤더에 보안 토큰 헤더(`X-Agent-Token`)를 포함하여 백엔드가 검증할 수 있도록 처리.

#### [NEW] [main.go](file:///h:/lee/go-watchdog/agent/main.go)
* 설정 로드 후 `time.Ticker`를 통해 지정된 수집 주기마다 `collector`와 `sender`를 호출하는 메인 루프 실행.

---

### 3. 백엔드 서버 (Server) 구성

#### [NEW] [db.go](file:///h:/lee/go-watchdog/server/db.go)
* SQLite3 DB 접속 및 기본 테이블 스키마 정의.
* 동시 읽기/쓰기 성능을 극대화하기 위해 커넥션 초기화 시 `PRAGMA journal_mode=WAL;` 설정을 활성화.
* 테이블 스키마:
  * `metrics` (id, hostname, cpu_usage, mem_total, mem_used, mem_percent, created_at)
  * `disk_metrics` (id, metric_id, path, total, used, percent)
* 백그라운드 고루틴으로 1시간에 한 번씩 보존 기간(14일)이 지난 데이터를 `DELETE`하는 자동 정리 로직 구현.

#### [NEW] [handler.go](file:///h:/lee/go-watchdog/server/handler.go)
* **`POST /api/metrics` 핸들러:** 에이전트로부터 전송된 JSON 데이터를 수집하여 DB에 인서트. 헤더의 `X-Agent-Token` 검증 미들웨어 탑재.
* **`GET /api/status` 핸들러:** 각 감시 대상 서버의 최신 자원 상태 리스트(JSON)를 반환. 수집된 지 30초 이상 경과한 서버는 `OFFLINE` 플래그를 추가로 지정.
* **`GET /` 핸들러:** 대시보드 HTML 파일 서빙.

#### [NEW] [main.go](file:///h:/lee/go-watchdog/server/main.go)
* 포트 `9090`을 열어 HTTP 라우팅 매핑 및 서버 가동. SQLite 연동 및 과거 데이터 자동 삭제 고루틴 실행.

#### [NEW] [dashboard.html](file:///h:/lee/go-watchdog/server/templates/dashboard.html)
* 모니터링 현황 관제 대시보드 웹 페이지.
* Vanilla JS의 `fetch`를 이용하여 5~10초 간격으로 `/api/status` API를 백그라운드 폴링(Polling) 호출.
* 각 서버별 CPU, RAM, Disk 상태를 차트/프로그레스바 형태 및 생동감 있는 UI 디자인으로 동적 갱신. 오프라인 장비는 빨간색 배지로 하이라이트 표시.

---

## Verification Plan

### Automated Tests
* **빌드 검증:**
  * 에이전트 컴파일: `go build -o agent.exe ./agent`
  * 백엔드 서버 컴파일: `go build -o server.exe ./server`
* **유닛 테스트:**
  * 시스템 메트릭 수집 및 DB 인서트 단위 테스트 코드 검증.

### Manual Verification
1. **서버 구동:** `server.exe`를 실행하여 `9090` 포트 가동 및 `monitoring.db` 파일 자동 생성 확인.
2. **에이전트 구동:** 테스트용 `config.json`을 설정하고 `agent.exe`를 실행하여 5초 주기로 백엔드로 데이터가 유입되는지 서버 콘솔 로그 확인.
3. **대시보드 접속:** 브라우저로 `http://localhost:9090` 접속 후 정상 수집되는 차트 데이터 확인.
4. **오프라인 검증:** 구동 중인 `agent.exe` 프로세스를 강제 종료한 뒤, 30초 후 대시보드에서 해당 에이전트의 상태가 `OFFLINE`으로 경고 표시되는지 확인.
