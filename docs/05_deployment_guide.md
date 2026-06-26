# [배포 가이드] Windows 서비스 등록 및 방화벽 설정

본 문서는 실무 운영 환경(Windows Server)에서 `go-watchdog` 서버 및 에이전트를 영구적이고 안정적으로 구동시키기 위한 방화벽 설정 및 윈도우 백그라운드 서비스 등록 방법을 다룹니다.

---

## 1. 서버 측 방화벽 포트 설정 (Inbound)

에이전트가 중심 백엔드 서버의 API(`9090` 포트)로 메트릭을 정상 전송하려면 서버 측 OS에서 포트 인바운드 차단을 해제해야 합니다.

### 1.1. GUI를 통한 방화벽 규칙 추가
1. **[제어판] -> [시스템 및 보안] -> [Windows Defender 방화벽] -> [고급 설정]**으로 이동합니다.
2. 왼쪽 트리 메뉴에서 **[인바운드 규칙]**을 클릭하고, 오른쪽 작업창에서 **[새 규칙...]**을 선택합니다.
3. **규칙 종류:** `포트(O)` 선택 후 [다음] 클릭.
4. **프로토콜 및 포트:** `TCP(T)` 및 `특정 로컬 포트(S)`에 `9090`을 입력하고 [다음] 클릭.
5. **작업:** `연결 허용(A)` 선택 후 [다음] 클릭.
6. **프로필:** `도메인`, `개인`, `공용` 모두 선택하고 [다음] 클릭.
7. **이름:** 규칙 이름을 `go-watchdog Ingress API`로 작성하고 [마침]을 클릭합니다.

### 1.2. PowerShell 커맨드로 즉시 설정
관리자 권한으로 열린 PowerShell에서 다음 커맨드를 실행하여 즉시 포트를 열 수 있습니다.
```powershell
New-NetFirewallRule -Name "GoWatchdogIngress" -DisplayName "go-watchdog Ingress API (TCP 9090)" -Description "Allow go-watchdog agents to report resource metrics." -Direction Inbound -LocalPort 9090 -Protocol TCP -Action Allow
```

---

## 2. 에이전트 Windows 백그라운드 서비스 등록

에이전트(`agent.exe`)가 윈도우 서버 재부팅 또는 세션 로그아웃 상황에 구애받지 않고 24시간 가동되도록 윈도우 백그라운드 서비스로 등록합니다. 서비스 등록에는 안전하고 검증된 유틸리티인 **NSSM(Non-Sucking Service Manager)** 사용을 적극 권장합니다.

### 2.1. NSSM 설치 및 준비
1. [NSSM 공식 웹사이트](https://nssm.cc/download)에서 운영체제 비트 수에 맞는 안정 버전을 다운로드합니다.
2. 다운로드한 압축 파일 내의 `nssm.exe`를 적절한 경로(예: `C:\nssm\nssm.exe` 또는 `agent.exe`와 같은 경로)에 배치합니다.
3. 배포 위치에 에이전트 실행 파일(`agent.exe`)과 설정 파일(`config.json`)을 다음과 같이 배치합니다.
   * 예: `C:\go-watchdog-agent\` 디렉토리 생성 후 `agent.exe`, `config.json`을 복사

### 2.2. 서비스 등록 (Command Line)
관리자 권한으로 PowerShell 또는 명령 프롬프트(CMD)를 실행하여 아래 명령어를 입력합니다.

```powershell
# 1. 서비스 등록 명령어 입력 (대화형 GUI 창이 활성화됨)
.\nssm.exe install GoWatchdogAgent

# 2. GUI 설정창 세팅
#  - Path: C:\go-watchdog-agent\agent.exe
#  - Startup directory: C:\go-watchdog-agent
#  - Arguments: -config C:\go-watchdog-agent\config.json
#  - [Install service] 버튼을 클릭하여 마무리합니다.
```

명령창에서 스크립트 기반으로 즉시 등록하려면 다음 명령어를 한 줄씩 실행합니다.
```powershell
.\nssm.exe install GoWatchdogAgent "C:\go-watchdog-agent\agent.exe"
.\nssm.exe set GoWatchdogAgent AppDirectory "C:\go-watchdog-agent"
.\nssm.exe set GoWatchdogAgent AppParameters "-config C:\go-watchdog-agent\config.json"
.\nssm.exe set GoWatchdogAgent Start SERVICE_AUTO_START
```

### 2.3. 서비스 시작 및 상태 확인
```powershell
# 서비스 시작
.\nssm.exe start GoWatchdogAgent

# 서비스 상태 조회
.\nssm.exe status GoWatchdogAgent
```
정상 작동 시 윈도우 서비스 관리자(`services.msc`) 창에서 `GoWatchdogAgent` 서비스가 **실행 중 (시작 유형: 자동)**으로 표시되는 것을 확인할 수 있습니다.

### 2.4. 서비스 중지 및 제거
에이전트 설정을 수정하거나 삭제할 필요가 있을 때 사용합니다.
```powershell
# 서비스 중지
.\nssm.exe stop GoWatchdogAgent

# 서비스 영구 제거
.\nssm.exe remove GoWatchdogAgent confirm
```

---

## 3. 로그 및 모니터링 확인

에이전트 서비스가 정상 동작하지 않는 경우 다음 순서로 확인합니다.
1. **NSSM 서비스 에러 로그 설정:**
   NSSM은 서비스 구동 로그를 파일로 리다이렉션해 줍니다. 서비스 등록 후 아래 설정을 적용하면 트러블슈팅이 쉬워집니다.
   ```powershell
   .\nssm.exe set GoWatchdogAgent AppStdout "C:\go-watchdog-agent\stdout.log"
   .\nssm.exe set GoWatchdogAgent AppStderr "C:\go-watchdog-agent\stderr.log"
   ```
2. **윈도우 이벤트 뷰어:**
   `[이벤트 뷰어] -> [Windows 로그] -> [응용 프로그램]` 탭에서 `nssm` 혹은 `GoWatchdogAgent` 관련 오류 기록을 확인합니다.
3. **방화벽 통신 차단 확인:**
   에이전트 구동 서버에서 수집 서버로의 TCP 포트 연결성을 확인합니다.
   ```powershell
   Test-NetConnection -ComputerName [수집서버IP] -Port 9090
   ```
   반환 결과가 `TcpTestSucceeded : True`여야 합니다.
