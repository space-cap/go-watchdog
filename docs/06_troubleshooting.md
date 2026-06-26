# [트러블슈팅 가이드] 장애 대응 및 자가 진단

`go-watchdog` 시스템 운영 중 발생할 수 있는 주요 에러 증상과 해결 방안을 정의한 문제 해결 문서입니다.

---

## 1. 데이터베이스 락 관련 오류 (`database is locked`)

### 1.1. 증상
에이전트가 성능 정보를 POST할 때, 서버 로그에 다음과 같은 오류가 발생하고 리포트가 누락됩니다.
```text
[Server] [Error] Failed to save metrics for ...: database is locked
```

### 1.2. 원인
SQLite는 단일 파일 기반 데이터베이스로, 여러 프로세스나 스레드가 동시에 쓰기(Write) 작업을 시도할 때 파일 잠금(Lock)이 일어날 수 있습니다.

### 1.3. 조치 방법
1. **WAL(Write-Ahead Logging) 모드 활성화 확인:**
   본 시스템은 `InitDB` 시점에 `PRAGMA journal_mode = WAL;`을 실행하도록 설계되어 있습니다. 데이터베이스 파일이 정상적으로 WAL 모드로 구동 중인지 확인하십시오.
   * `monitoring.db-wal` 및 `monitoring.db-shm` 임시 파일이 생성되어 구동 중인지 확인합니다.
2. **Busy Timeout 확인:**
   커넥션 설정 시 `PRAGMA busy_timeout = 5000;` 옵션이 정상 동작하여 락 발생 시 최대 5초간 대기 후 다시 시도하는지 확인합니다.
3. **서버 인스턴스 중복 구동 여부:**
   하나의 `monitoring.db` 파일에 대해 두 개 이상의 `server.exe` 프로세스가 가동 중인지 확인하고, 중복 프로세스를 종료하십시오.

---

## 2. API 인증 오류 (`Unauthorized: Invalid or missing X-Agent-Token`)

### 2.1. 증상
에이전트 로그에 다음과 같이 HTTP 전송 실패 메시지가 출력됩니다.
```text
[Agent] [Error] Failed to push metrics: server responded with status: 401 Unauthorized
```

### 2.2. 원인
에이전트가 헤더에 첨부하는 인증 토큰(`X-Agent-Token`)과 수집 서버에 설정된 인증 키(`-token` 인자)가 불일치하여 보안 필터에서 차단된 경우입니다.

### 2.3. 조치 방법
1. **에이전트 설정 확인:**
   `agent/config.json`의 `"auth_token"` 값이 수집 서버 구동 시 사용한 토큰 파라미터와 일치하는지 확인합니다.
2. **서버 시작 스크립트 확인:**
   서버 구동 시 명시한 `-token` 파라미터 값을 확인하십시오. 파라미터를 입력하지 않은 경우 기본값은 `watchdog-secret-token` 입니다.

---

## 3. 에이전트의 오프라인(OFFLINE) 오탐지

### 3.1. 증상
에이전트 서버가 정상적으로 살아있고 프로세스도 작동 중인데 대시보드 상에서 `OFFLINE` 상태로 경고가 출력됩니다.

### 3.2. 원인
1. 에이전트 구동 서버와 수집 서버 간의 **시스템 시간(NTP) 불일치**.
2. 에이전트 수집 주기(`interval_seconds`) 대비 수집 서버의 통신 지연.
3. 에이전트가 리포팅을 시도하는 도중 네트워크 통신 일시 장애.

### 3.3. 조치 방법
1. **시간 동기화:**
   에이전트와 서버가 위치한 장비의 시스템 시각이 표준 시각(NTP 서버 동기화)과 일치하는지 확인합니다. 윈도우 시간 동기화 명령을 수행합니다.
   ```powershell
   w32tm /resync
   ```
2. **수집 주기 완화:**
   에이전트의 수집 주기를 늘린 경우, 수집 서버가 판단하는 임계값(현재 마지막 리포트 시간 기준 30초 초과 시 오프라인 판정)도 완화되어야 합니다. 필요시 `server/handler.go`의 오프라인 판단 임계 시간(기본 30초)을 소스에서 수정 후 재빌드하십시오.

---

## 4. Windows 성능 정보 수집 오류 (`0.0%` 고정 등)

### 4.1. 증상
에이전트 로그 또는 대시보드에 CPU 사용률이 계속 `0.0%`로 표시되거나 디스크 정보를 읽어오지 못합니다.

### 4.2. 원인
1. **Windows 성능 카운터(Performance Counters) 손상:** Windows OS 내부의 WMI 카운터가 손상된 경우 `gopsutil` 라이브러리가 값을 가져오지 못합니다.
2. **권한 부족:** 시스템 볼륨 정보나 특정 드라이브에 대한 접근 권한 제한.

### 4.3. 조치 방법
1. **Windows 성능 카운터 복구:**
   관리자 권한의 PowerShell에서 다음 명령어를 실행하여 Windows 성능 카운터를 재구축합니다.
   ```powershell
   lodctr /R
   wmiadm /synclodctr
   winmgmt /resetsubsystem
   ```
2. **에이전트 관리자 권한 실행:**
   에이전트 서비스를 일반 사용자 계정으로 구동 중인 경우, `Local System` 계정 또는 관리자 권한을 가진 계정으로 동작하도록 NSSM 설정을 수정합니다.
