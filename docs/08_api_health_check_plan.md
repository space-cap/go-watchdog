# [작업 계획서] REST API 헬스체크 모니터링 및 알림 시스템 구축 (리소스 분리 반영)

본 문서는 `go-watchdog` 서버에 외부 REST API(예: 포트 5173, 8910 등의 헬스체크 엔드포인트)를 등록하고, 이를 주기적으로 체크하여 상태를 실시간으로 관제하며, 장애 발생 시 슬랙(Slack) 및 디스코드(Discord)로 즉각 알림을 발송하는 시스템 구축을 위한 작업 계획서입니다. 

특히, 대시보드 코드의 비대화를 방지하고 유지보수성을 극대화하기 위해 HTML, CSS, JS 리소스를 물리적으로 분리하여 서빙하는 설계를 반영했습니다.

---

## 1. 개요 및 목적
* **배경:** 현재 관리 중인 서버 및 서비스가 약 10개 정도이며, 장애 발생 시 인지 시간을 최소화하기 위해 슬랙 및 디스코드 채널로 실시간 알림을 받아야 합니다.
* **목적:** 
  * 헬스체크 대상 API 정보(URL, 체크 주기 등)를 동적으로 등록/삭제할 수 있는 관리 기능 제공.
  * 서버 내부의 스케줄러를 통해 등록된 API에 주기적인 HTTP 요청을 보내 상태를 측정.
  * **상태 전이 감지 로직:** 정상(`ONLINE`) -> 장애(`OFFLINE`) 또는 장애(`OFFLINE`) -> 정상(`ONLINE`) 복구 시에만 슬랙/디스코드 웹훅(Webhook)을 통해 알림을 발송하여 무분별한 알림 스팸 방지.
  * **프론트엔드 리소스 분리:** 기존 단일 대시보드 파일(`dashboard.html`)을 HTML, CSS, JS 파일로 쪼개어 가독성을 높이고 코드 비대화 방지.

---

## 2. 시스템 아키텍처 및 데이터 흐름

```text
+---------------------------------------------------------------------------------+
|                                 go-watchdog Server                              |
|                                                                                 |
|  +---------------------------------------------------------------------------+  |
|  |                             Web Dashboard Resources                       |  |
|  |  +------------------+  +------------------+  +-------------+  +--------+  |  |
|  |  |  dashboard.html  |  |    style.css     |  |dashboard.js |  |health.js|  |  |
|  |  |   (HTML 구조)    |  |   (공통 스타일)  |  | (리소스폴링)|  |(헬스체크)|  |  |
|  |  +--------+---------+  +--------+---------+  +------+------+  +----+---+  |  |
|  +-----------|---------------------|-------------------|--------------|------+  |
|              v                     v                   v              v         |
|              +---------------------+-------------------+--------------+         |
|                                    | (Static Router: /static/*)                 |
|                                    v                                            |
|                        +-----------+------------+        +-------------------+  |
|                        |      API Handlers      |        |Health Check Runner|  |
|                        |  (CRUD Targets/Status) |        |   (Go Goroutine)  |  |
|                        +-----------+------------+        +--------+----------+  |
|                                    |                              |             |
|                      (Read/Write)  v                              v (GET Ping)  |
|                            +---------------+             +--------+----------+  |
|                            | monitoring.db |             |  Target REST APIs |  |
|                            |   (SQLite)    |             | - localhost:5173  |  |
|                            +---------------+             | - localhost:8910  |  |
|                                                           +--------+----------+  |
|                                                                    |             |
|                                                     (Status Change)v             |
|                                                          +--------+----------+  |
|                                                          |Notification Sender|  |
|                                                          | (Slack / Discord) |  |
|                                                          +-------------------+  |
+---------------------------------------------------------------------------------+
```

1. **정적 리소스 서빙:** Go의 `go:embed` 기능을 이용하여 `templates/` 하위 폴더의 모든 자원을 바이너리에 내장하고, `/static/` 라우터를 통해 각 리소스를 분리 서빙합니다.
2. **대상 등록:** 사용자가 대시보드 웹 UI에서 API URL과 체크 주기를 입력하여 등록하면 `health_targets` 테이블에 저장됩니다.
3. **주기적 헬스체크:** 백엔드 서버 기동 시 **Health Check Runner (고루틴)**가 실행되어, DB에 저장된 대상들에게 개별 주기마다 비동기 HTTP GET 요청을 전송합니다.
4. **상태 전이 및 알림 판단:** 이전 상태값과 현재 측정 상태값을 비교하여 `ONLINE <-> OFFLINE` 상태 변화가 일어난 시점에만 슬랙/디스코드 웹훅을 통해 알림을 발송합니다.
5. **결과 기록:** HTTP 응답 코드, 지연 시간(Latency, ms), 에러 발생 여부를 측정하여 `health_logs` 테이블에 삽입합니다.

---

## 3. 데이터베이스 설계 (SQLite)

경량성과 성능 관리를 위해 기존 `monitoring.db` 파일에 두 개의 테이블을 추가로 정의합니다.

### 3.1 헬스체크 대상 관리 테이블 (`health_targets`)
```sql
CREATE TABLE IF NOT EXISTS health_targets (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,                         -- API 서비스 식별 명칭 (예: "Front Web App")
    url TEXT NOT NULL UNIQUE,                   -- 헬스체크 대상 API URL (예: "http://localhost:5173/api/health")
    interval_seconds INTEGER NOT NULL DEFAULT 60, -- 헬스체크 수행 주기 (초 단위)
    timeout_seconds INTEGER NOT NULL DEFAULT 5,  -- HTTP 요청 타임아웃 (초 단위)
    is_active INTEGER NOT NULL DEFAULT 1,       -- 활성화 여부 (1: 활성, 0: 일시정지)
    created_at DATETIME NOT NULL                -- 등록 시간
);
```

### 3.2 헬스체크 이력 로그 테이블 (`health_logs`)
* 데이터가 무한히 쌓여 용량을 차지하지 않도록, 기존 자원 수집 메트릭과 동일한 **14일 보존 정책** 및 **Cascading Delete**가 적용됩니다.
```sql
CREATE TABLE IF NOT EXISTS health_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    target_id INTEGER NOT NULL,
    status_code INTEGER,                        -- 수신된 HTTP 응답 코드 (연결 실패 시 NULL)
    latency_ms INTEGER,                         -- 응답 지연 속도 (밀리초)
    is_success INTEGER NOT NULL,                -- 성공 여부 (1: 성공, 0: 실패)
    error_message TEXT,                         -- 연결 실패 혹은 타임아웃 에러 메시지
    timestamp DATETIME NOT NULL,                -- 체크 수행 시각
    FOREIGN KEY(target_id) REFERENCES health_targets(id) ON DELETE CASCADE
);

-- 최신 로그 조회를 최적화하기 위한 인덱스 추가
CREATE INDEX IF NOT EXISTS idx_health_logs_target_time ON health_logs(target_id, timestamp DESC);
```

---

## 4. 백엔드 설정 확장 및 알림 명세

### 4.1 `server/config.json` 설정 파일 확장
알림을 발송할 슬랙 및 디스코드의 웹훅 URL을 관리자 설정 파일에 추가로 구성합니다.
```json
{
  "port": 9090,
  "auth_token": "watchdog-secret-token",
  "db_path": "monitoring.db",
  "retention_days": 14,
  "slack_webhook_url": "https://hooks.slack.com/services/YOUR_WORKSPACE_ID/YOUR_CHANNEL_ID/YOUR_TOKEN",
  "discord_webhook_url": "https://discord.com/api/webhooks/YOUR_WEBHOOK_ID/YOUR_WEBHOOK_TOKEN"
}
```

### 4.2 알림 메시지 포맷 예시
* **🚨 장애 발생 알림 (Alert)**
  > **[go-watchdog] 서비스 장애 감지**
  > * **서비스명:** Front Web (Vite)
  > * **상태:** `OFFLINE` (장애)
  > * **대상 URL:** http://localhost:5173/api/health
  > * **에러 메시지:** `dial tcp [::1]:5173: connectex: No connection could be made...`
  > * **발생 시각:** 2026-07-10 15:42:10 KST
* **✅ 정상 복구 알림 (Recovery)**
  > **[go-watchdog] 서비스 복구 완료**
  > * **서비스명:** Front Web (Vite)
  > * **상태:** `ONLINE` (정상)
  > * **대상 URL:** http://localhost:5173/api/health
  > * **응답 속도:** 12ms (HTTP 200)
  > * **복구 시각:** 2026-07-10 15:43:05 KST

---

## 5. UI/UX 화면 설계 및 리소스 분리

### 5.1 리소스 분리 구성안 (Directory Structure)
기존 `server/templates/dashboard.html`에 밀집되어 있던 코드를 다음과 같이 4개 파일로 쪼갭니다.
1. **`dashboard.html`:** 순수 HTML 구조만 존재 (약 150라인으로 대폭 축소)
2. **`style.css`:** 다크 모드 테마, 글래스모피즘, 카드 그리드 레이아웃, 애니메이션 등 CSS 전체 정의
3. **`dashboard.js`:** 기존 에이전트 서버 상태 폴링, 차트 업데이트 관련 JS 로직
4. **`health.js`:** [NEW] 헬스체크 대상 CRUD 비동기 요청, 헬스체크 상태 폴링, 모달 다이얼로그 제어 로직

### 5.2 대시보드 탭 통합 레이아웃
상단 헤더 영역 아래에 **탭 네비게이션(Tab Navigation)**을 추가하여 두 화면을 유기적으로 전환할 수 있게 설계합니다.
* **[탭 1] 서버 리소스 모니터링:** 기존의 에이전트 자원 모니터링 그리드 카드 노출.
* **[탭 2] 서비스 API 헬스체크:** 신규 헬스체크 관리 및 모니터링 화면 노출.

---

## 6. 상세 구현 계획

### [Phase 0] 프론트엔드 리소스 분리 리팩토링 (사전 작업)
* **파일 분리:** 기존 `dashboard.html` 내부의 `<style>` 태그와 `<script>` 태그 영역을 각각 `style.css`와 `dashboard.js`로 추출하여 생성.
* **Go 임베드 매핑:** `server/handler.go` 및 `server/main.go`에서 `//go:embed templates/*` 처리를 진행하고 `/static/` 라우터를 추가하여 정적 자원을 서빙하도록 구성.
* **검증:** 파일 분리 이후 기존의 대시보드 화면이 깨지지 않고 정상 동작하는지 먼저 빌드 및 화면 확인 진행.

### [Phase 1] DB 마이그레이션 및 공통 로직 개발
* `server/db.go` 내에 `health_targets` 및 `health_logs` 테이블 생성 DDL 추가.
* 프로그램 시작 시 `InitDB` 과정에서 테이블 자동 생성 검증.
* 보존 기한이 지난 로그를 함께 지워주는 자동 Retention 정리 함수에 `health_logs` 테이블 연동.

### [Phase 2] 백그라운드 헬스체크 및 알림 모듈 구현
* `server/health_runner.go` 신설:
  * 데이터베이스의 타겟을 조회하여 고루틴 채널이나 멀티 타이머를 사용해 개별 주기에 따라 동작하게 구현.
  * HTTP 요청 수행 시 리소스 누수가 없도록 `http.Client` 커스텀 생성 및 `timeout` 지정.
  * **상태 전이 감지 로직:** 메모리 맵(Memory Map) 혹은 DB 직전 로그 레코드를 조회해 이전 상태와 비교하여 알림 트리거 활성화.
* `server/notifier.go` 신설:
  * `config.json`의 웹훅 URL 정보를 로드하여 Slack과 Discord 형식에 맞는 JSON 페이로드 빌드 및 전송.

### [Phase 3] API 컨트롤러 엔드포인트 구현
* `server/handler.go`에 타겟 등록(`POST`), 삭제(`DELETE`), 조회(`GET`) 핸들러 추가.
* `server/main.go`에 신규 핸들러 라우팅 매핑.

### [Phase 4] 프론트엔드 UI 연동
* `server/templates/dashboard.html` 내부에 탭 전환 스크립트 및 API 관리 UI 템플릿(HTML/CSS) 통합.
* `server/templates/health.js` 신규 생성: Vanilla JS로 등록 모달 열기/닫기 및 API 비동기 `fetch` 통신 구현.
* 주기적 상태 폴링 루프(Polling Loop)를 구동하여 상태 변화 실시간 반영.

---

## 7. 검증 및 테스트 시나리오

1. **정적 파일 분리 검증:** 분리된 CSS, JS 파일이 브라우저에서 `/static/style.css`, `/static/dashboard.js` 경로로 정상 호출되고 에러가 없는지 검증.
2. **설정 검증:** `config.json`에 테스트용 슬랙/디스코드 웹훅 URL 입력 후 정상 구동 확인.
3. **모의 타겟 등록 테스트:**
   - 헬스체크 등록 모달을 통해 `http://localhost:5173/api/health` 및 `http://localhost:8910/api/health`를 수동으로 등록.
4. **장애 발생 및 알림 검증:**
   - 실행 중인 모의 API 서버를 강제 종료 -> 대시보드 상태 `OFFLINE` 감지 -> **슬랙/디스코드 채널로 🚨 장애 알림이 즉시 1회 발송되는지 확인**.
   - 이후 주기적 체크가 일어나더라도 추가 스팸 알림이 발송되지 않는지 확인.
5. **복구 및 알림 검증:**
   - 모의 API 서버를 재구동 -> 대시보드 상태 `ONLINE` 복구 감지 -> **슬랙/디스코드 채널로 ✅ 복구 완료 알림이 즉시 1회 발송되는지 확인**.
