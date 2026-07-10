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

---

## 4. Oracle Cloud Infrastructure (Ubuntu) 배포 가이드

오라클 클라우드(OCI)의 우분투 가상 머신(VM) 환경에 `go-watchdog` 수집 서버를 배포하고 영구 가동하기 위한 방화벽 및 시스템 서비스 설정 가이드입니다.

### 4.1. Linux용 바이너리 크로스 컴파일
개발 PC(Windows) 환경에서 Linux(Ubuntu) 환경으로 배포하기 위한 실행 파일을 빌드합니다.
PowerShell 세션에서 다음 명령어를 실행합니다.

```cmd
:: 1. 리눅스 환경 변수 설정
set GOOS=linux
set GOARCH=amd64

:: 2. 수집 서버 빌드 (bin/server 바이너리 파일 생성)
go build -o bin/server ./server

:: 3. 자원 수집 에이전트 빌드 (bin/agent 바이너리 파일 생성)
go build -o bin/agent ./agent

:: 4. 환경 변수 초기화 (선택 사항)
set GOOS=
set GOARCH=
```

#### 2) Windows 10 PowerShell에서 빌드할 때
PowerShell 창을 열고 프로젝트 루트 디렉토리에서 아래 명령어를 실행합니다.

```powershell
$env:GOOS="linux"
$env:GOARCH="amd64"
go build -o bin/server ./server
go build -o bin/agent ./agent
$env:GOOS=""
$env:GOARCH=""
```

#### 3) 빌드된 파일을 SCP 명령어로 우분투 서버에 전송
윈도우 10 CMD 또는 PowerShell에서 `scp` 명령어를 이용하여 빌드 완료된 리눅스용 바이너리를 우분투 서버의 지정 디렉토리(`~/go-watchdog/`)로 다이렉트 전송할 수 있습니다. (오라클 클라우드의 SSH 접속용 프라이빗 키 `.key` 또는 `.pem` 파일 경로를 적어줍니다.)

```cmd
:: 윈도우 CMD/PowerShell에서 우분투 서버의 사용자 홈 디렉토리 경로로 전송
scp -i "C:\path\to\your-oracle-key.pem" bin/server ubuntu@<우분투_서버_IP>:~/go-watchdog/server
scp -i "C:\path\to\your-oracle-key.pem" bin/agent ubuntu@<우분투_서버_IP>:~/go-watchdog/agent
```

---

### 4.2. 우분투 서버에서 배포 및 덮어쓰기 적용

우분투 서버에서 이미 수집 서버(`server`)나 에이전트(`agent`)가 실행 중인 상태에서는 파일이 실행 상태로 잠겨 있어 덮어쓰기 시 `text file busy` 오류가 발생합니다. 따라서 반드시 실행 중인 기존 프로세스를 중지한 뒤 덮어써야 합니다.

#### 1) 기존 구동 중인 프로세스 중지
* **systemd 서비스로 구동 중인 경우:**
  ```bash
  sudo systemctl stop go-watchdog
  ```
* **수동으로 백그라운드(`nohup`) 실행 중인 경우:**
  ```bash
  pkill -f server
  # 또는
  killall server
  ```

#### 2) 실행 권한 부여 및 덮어쓰기
전송받은 바이너리를 덮어쓴 후 실행이 가능하도록 실행 권한을 명시적으로 부여합니다.
```bash
# 1. 전송 받은 실행 바이너리에 실행 권한 부여
chmod +x ~/go-watchdog/server
chmod +x ~/go-watchdog/agent

# 2. 만약 /opt/go-watchdog/ 등 시스템 공용 경로에 배포하여 구동하는 환경인 경우 덮어쓰기 복사 진행
sudo cp ~/go-watchdog/server /opt/go-watchdog/server
sudo cp ~/go-watchdog/agent /opt/go-watchdog/agent
```

#### 3) 프로세스 재기동
* **systemd 서비스로 구동하는 경우:**
  ```bash
  sudo systemctl start go-watchdog
  ```
* **수동으로 백그라운드(`nohup`) 실행하는 경우:**
  ```bash
  cd ~/go-watchdog
  nohup ./server -config config.json > server.log 2>&1 &
  ```

---

### 4.3. Oracle Cloud 인프라 보안 규칙(방화벽) 설정
오라클 클라우드는 기본적으로 가상 네트워크(VCN) 인프라 레벨에서 포트가 차단되어 있습니다. 포트 허용 규칙을 가장 먼저 구성해야 합니다.

1. **오라클 클라우드 콘솔**에 로그인합니다.
2. 배포 대상 인스턴스 상세 정보 화면으로 이동하여 **[기본 가상 클라우드 네트워크(VCN)]** 링크를 클릭합니다.
3. 왼쪽 메뉴에서 **[보안 목록(Security Lists)]**을 선택하고, 사용 중인 기본 보안 목록을 클릭합니다.
4. **[수신 규칙 추가(Add Ingress Rules)]** 버튼을 클릭합니다.
5. 아래와 같이 설정한 뒤 **[수신 규칙 추가]**를 클릭합니다.
   * **소스 유형:** `CIDR`
   * **소스 CIDR:** `0.0.0.0/0` (또는 에이전트들이 위치한 특정 IP 대역)
   * **IP 프로토콜:** `TCP`
   * **대상 포트 범위:** `9090` (설정한 포트 번호)
   * **설명:** `go-watchdog web and ingest api port`

### 4.3. Ubuntu OS 내부 방화벽 설정
오라클 클라우드의 우분투 이미지는 기본적으로 `iptables` 규칙이 엄격하게 바인딩되어 있어, 단순히 `ufw`를 켜는 것만으로는 포트가 열리지 않습니다. OS 내부에서 아래 명령어를 실행하여 포트를 완전 허용해 주어야 합니다.

```bash
# OS 내부로 SSH 접속 후 실행
# 1. iptables 규칙의 수신 허용 목록에 9090 포트 추가 (우선순위 상위에 적용하기 위해 -I INPUT 6 사용)
sudo iptables -I INPUT 6 -p tcp --dport 9090 -j ACCEPT

# 2. 변경된 iptables 규칙 영구 저장 (재부팅 시 초기화 방지)
sudo netfilter-persistent save
```
> [!NOTE]
> 만약 서버에서 `ufw` 방화벽을 주로 사용 중인 환경이라면 아래 명령어를 추가로 수행합니다.
> ```bash
> sudo ufw allow 9090/tcp
> ```

### 4.4. systemd 서비스 등록 및 백그라운드 영구 구동
서버 재부팅 시에도 자동으로 `go-watchdog` 서버가 가동되고 백그라운드에서 상시 돌도록 Linux 표준 데몬 관리인 `systemd` 서비스로 등록합니다.

#### 1) 배포 디렉토리 세팅 및 실행 권한 부여
```bash
# 1. 서비스가 동작할 전용 디렉토리 생성 및 설정 파일 복사
sudo mkdir -p /opt/go-watchdog
sudo cp server/config.json /opt/go-watchdog/config.json

# 2. 컴파일하여 전송한 server 실행 바이너리를 디렉토리로 이동 및 실행 권한 추가
sudo mv server /opt/go-watchdog/server
sudo chmod +x /opt/go-watchdog/server
```

#### 2) systemd 서비스 파일 작성
`/etc/systemd/system/go-watchdog.service` 파일을 생성하고 아래 설정을 입력합니다.

```ini
[Unit]
Description=Go-Watchdog Resource and Health Monitor Server
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=/opt/go-watchdog
ExecStart=/opt/go-watchdog/server -config /opt/go-watchdog/config.json
Restart=always
RestartSec=5
StandardOutput=append:/var/log/go-watchdog.log
StandardError=append:/var/log/go-watchdog.err.log

[Install]
WantedBy=multi-user.target
```

#### 3) 서비스 활성화 및 시작
```bash
# 1. 새 서비스 파일 등록 반영
sudo systemctl daemon-reload

# 2. 서비스 시작 및 상태 확인
sudo systemctl start go-watchdog.service
sudo systemctl status go-watchdog.service

# 3. 서버 부팅 시 자동 시작 등록
sudo systemctl enable go-watchdog.service
```

#### 4) 서비스 제어 명령어 목록
```bash
# 서비스 상태 확인
sudo systemctl status go-watchdog

# 서비스 중지
sudo systemctl stop go-watchdog

# 서비스 재시작 (설정 변경 시 등)
sudo systemctl restart go-watchdog

# 로그 실시간 관제
tail -f /var/log/go-watchdog.log
```
