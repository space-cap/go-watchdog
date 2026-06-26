# [빌드 가이드] Windows & Ubuntu OS별 빌드 매뉴얼

`go-watchdog` 시스템의 수집 서버(`server`) 및 에이전트(`agent`)를 각 운영체제(Windows, Ubuntu) 환경에 맞춰 빌드하는 가이드라인입니다.

Go의 뛰어난 **크로스 컴파일(Cross-compilation)** 기능을 활용하여, 현재 OS(예: Windows)에서 대상 OS(예: Ubuntu Linux)용 실행 바이너리를 생성하는 방법도 포함합니다.

---

## 1. 사전 준비 (Go 설치)

빌드를 수행하는 빌드 머신에는 Go SDK가 설치되어 있어야 합니다.

### 1.1. Windows 환경
1. [Go 공식 다운로드 페이지](https://go.dev/dl/)에서 Windows 설치 프로그램(`.msi`)을 다운로드하여 설치합니다.
2. 설치 경로(예: `C:\Go\bin` 혹은 `H:\Program Files\Go\bin`)를 시스템 환경 변수 `PATH`에 등록합니다.

### 1.2. Ubuntu 환경
터미널에서 아래 명령어를 실행하여 Go를 설치합니다.
```bash
sudo apt update
sudo apt install golang-go -y

# 설치 확인
go version
```

---

## 2. Windows 환경에서 빌드하기

### 2.1. 로컬 빌드 (Windows용 실행 파일 `.exe` 생성)
프로젝트 루트 디렉토리(`go-watchdog`)에서 PowerShell을 열고 실행합니다.

```powershell
# 1. 터미널 세션에 Go PATH 지정 (Go 명령어가 동작하지 않는 경우에만 실행)
$env:Path = "H:\Program Files\Go\bin;" + $env:Path

# 2. 의존성 패키지 동기화
go mod tidy

# 3. 수집 서버 빌드 (server.exe 생성)
go build -o server.exe ./server

# 4. 자원 수집 에이전트 빌드 (agent.exe 생성)
go build -o agent.exe ./agent
```

### 2.2. 크로스 컴파일 (Windows에서 Ubuntu/Linux용 바이너리 생성)
Windows에서 빌드하되, 실제 구동할 대상 서버가 Ubuntu인 경우 다음 명령어를 실행합니다. CGO가 필요 없는 코드이므로 컴파일러 설치 없이 즉시 실행 가능합니다.

```powershell
# 터미널 세션에 Go PATH 지정 (필요시)
$env:Path = "H:\Program Files\Go\bin;" + $env:Path

# Linux 실행 환경(GOOS=linux, GOARCH=amd64)으로 설정 후 빌드
$env:GOOS="linux"
$env:GOARCH="amd64"

# 서버 빌드 (확장자 없음)
go build -o server ./server

# 에이전트 빌드 (확장자 없음)
go build -o agent ./agent

# 빌드 완료 후 환경 변수 원상 복구 (기본값 Windows)
$env:GOOS=""
$env:GOARCH=""
```

---

## 3. Ubuntu 환경에서 빌드하기

### 3.1. 로컬 빌드 (Ubuntu/Linux용 바이너리 생성)
Ubuntu 서버 터미널에서 소스코드를 다운로드(또는 복사)한 뒤 프로젝트 루트 경로에서 아래 명령어를 실행합니다.

```bash
# 1. 의존성 다운로드 및 정리
go mod tidy

# 2. 수집 서버 빌드 (실행 파일: server)
go build -o server ./server

# 3. 자원 수집 에이전트 빌드 (실행 파일: agent)
go build -o agent ./agent

# 4. 실행 권한 부여
chmod +x server agent
```

### 3.2. 크로스 컴파일 (Ubuntu에서 Windows용 실행 파일 `.exe` 생성)
Ubuntu 머신에서 빌드하되, 실제 구동할 대상 서버가 Windows인 경우 다음 환경 변수를 주입하여 빌드합니다.

```bash
# Windows 실행 환경(GOOS=windows, GOARCH=amd64) 설정 주입하여 빌드
GOOS=windows GOARCH=amd64 go build -o server.exe ./server
GOOS=windows GOARCH=amd64 go build -o agent.exe ./agent
```

---

## 4. 빌드 결과물 정리

| 빌드 대상 OS | 산출 파일 | 배포 및 구동 대상 |
| :--- | :--- | :--- |
| **Windows** | `server.exe` <br> `agent.exe` | 대시보드 구동용 서버 <br> 자원 감시 대상 Windows PC/서버 |
| **Ubuntu** | `server` <br> `agent` | 대시보드 구동용 Linux 서버 <br> 자원 감시 대상 Linux PC/서버 |

* **주의:** `server` 또는 `server.exe`는 대시보드 웹 템플릿(`templates/dashboard.html`)을 내장(`go:embed`)하고 있으므로, 배포 시 별도의 HTML 파일 전송 없이 **단일 바이너리 파일 하나만** 대상 서버로 복사하여 독립적으로 구동하면 됩니다.
