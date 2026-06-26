# go-watchdog

운영 중인 Windows 서버의 성능 자원(CPU, Memory, Disk)을 가볍고 신속하게 수집하여 모니터링할 수 있는 **초경량 실시간 서버 상태 감시 시스템**입니다.

---

## 1. 시스템 구조 및 흐름

본 시스템은 에이전트가 백엔드 서버로 직접 데이터를 리포트하는 **Agent Push** 구조를 따르고 있어, 수집 서버 측의 포트 하나만 활성화하면 통신이 가능합니다.

```text
+------------------------+                     +---------------------------+
|  Monitored Windows VM  |                     |    Central Ingest Server  |
|  +------------------+  |   HTTP POST /json   |  +---------------------+  |
|  |    agent.exe     |--|-------------------->|  |     server.exe      |  |
|  | (gopsutil logic) |  |  (Header Token Auth)|  | (database/sql, embed) |  |
|  +------------------+  |                     +-----------+-------------+  |
+------------------------+                                 |               |
                                                           v               v
                                                    +-------------+  +-----------+
                                                    | monitoring  |  | dashboard |
                                                    |    .db      |  |   (HTML)  |
                                                    +-------------+  +-----------+
```

---

## 2. 주요 특징

* **초경량 성능:** 가상 머신(JVM 등)이나 에이전트 구동 런타임이 필요 없는 Go 바이너리 단독 구동.
* **배포 단순화:** `go:embed` 기능을 이용하여 HTML 대시보드 코드를 실행 파일(`server.exe`) 내부에 내장, 파일 하나로 서버와 웹 UI가 함께 실행됩니다.
* **고성능 로컬 저장소:** SQLite3에 WAL(Write-Ahead Logging) 모드 및 비동기 쓰기를 적용하여 데이터의 안정적 수집 보장.
* **자동 보존 정책:** 14일 이전의 만료된 메트릭 데이터를 백그라운드 스레드에서 자동으로 제거(Cascading Delete)하여 디스크 공간을 확보합니다.
* **실시간 대시보드:** Vanilla JS 비동기 Polling을 이용한 미려한 다크 모드 및 글래스모피즘 웹 관제 화면 제공.

---

## 3. 빌드 방법

개발 환경에 Go 런타임이 설치되어 있어야 합니다.

### 3.1. 수집 서버 빌드
```powershell
# Go bin 경로를 세션 PATH에 추가 (설치 경로가 다를 시 변경)
$env:Path = "H:\Program Files\Go\bin;" + $env:Path

# 서버 바이너리 빌드
go build -o server.exe ./server
```

### 3.2. 에이전트 빌드
```powershell
# 에이전트 바이너리 빌드
go build -o agent.exe ./agent
```

---

## 4. 구동 가이드

### 4.1. 서버 실행
서버는 수집 포트(`9090`), 보안 토큰값, DB 경로 및 데이터 보존 기한을 파라미터로 받아 시작합니다.
```powershell
# 기본 설정값으로 구동 (포트 9090, monitoring.db 생성)
.\server.exe

# 사용자 정의 인자로 구동
.\server.exe -port 9090 -token watchdog-secret-token -db monitoring.db -retention 14
```

### 4.2. 에이전트 설정 및 실행
에이전트 구동 파일과 동일 경로에 `config.json`을 위치시킵니다.

* **설정 템플릿 (`agent/config.json`):**
```json
{
  "agent_id": "windows-db-server-01",
  "server_url": "http://localhost:9090/api/metrics",
  "auth_token": "watchdog-secret-token",
  "interval_seconds": 5
}
```

* **실행:**
```powershell
# 에이전트 실행 (기본값 config.json 로드)
.\agent.exe

# 다른 경로의 설정 파일 지정 시
.\agent.exe -config agent/config.json
```

에이전트가 가동되면 콘솔에 수집 보고 메시지가 출력되며, 웹 브라우저로 `http://localhost:9090`에 접속하여 실시간 차트를 조회할 수 있습니다.
