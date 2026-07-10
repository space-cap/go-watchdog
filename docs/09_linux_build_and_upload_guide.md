# [배포 매뉴얼] Windows CMD 환경에서의 리눅스 빌드 및 업로드 가이드

본 문서는 Windows 10 개발 PC의 **명령 프롬프트(CMD)** 환경에서 수집 서버(`server`)를 Linux(Ubuntu)용으로 크로스 컴파일 빌드하고, SCP 전송을 통해 오라클 클라우드 원격 서버로 배포 및 안전 교체(Hot-swap)하는 단계별 가이드를 제공합니다.

---

## 1. 사전 준비 사항
* **개발 PC OS:** Windows 10 (명령 프롬프트 CMD 사용)
* **SSH 프라이빗 키 경로:** `h:\ssh-key-2026-06-19.key`
* **우분투 서버 IP:** `140.245.64.172`
* **우분투 서버 계정:** `ubuntu`
* **배포 대상 디렉토리:** `~/go-watchdog/`

---

## 2. 배포 및 업데이트 단계 (Step-by-Step)

### Step 1. 우분투 서버에서 기존 프로세스 확인 및 종료
리눅스는 실행 중인 파일이 프로세스 메모리에 로드되어 있는 동안 쓰기 락(Lock)을 걸어둡니다. 이 상태에서 파일 덮어쓰기(`scp`)를 시도하면 `text file busy` 또는 `Failure` 에러가 발생하므로 반드시 기존 프로세스를 먼저 종료해야 합니다.

1. **우분투 SSH 터미널에서 구동 중인 프로세스 조회:**
   ```bash
   ps -ef | grep server
   ```
2. **해당 프로세스 ID(PID) 종료:**
   * 출력 내용 중 `go-watchdog` 서버의 프로세스 ID(예: `132206`)를 확인하고 종료시킵니다.
   ```bash
   kill 132206
   ```
   * 프로세스 이름 단위로 깔끔하게 종료하고 싶은 경우 아래 명령을 수행합니다:
   ```bash
   killall server
   ```

---

### Step 2. Windows CMD에서 리눅스용 크로스 컴파일 빌드
Windows 10 CMD 창을 열고 프로젝트 루트 경로(`H:\lee\go-watchdog`)로 이동하여 리눅스 환경에 최적화된 실행 파일을 빌드합니다.

```cmd
:: 1. 리눅스 64비트 환경 대상 컴파일 지정 (CMD용 set 명령어)
set GOOS=linux
set GOARCH=amd64

:: 2. 수집 서버 빌드 (bin/server 바이너리 파일 생성)
go build -o bin/server ./server

:: 3. (선택) 에이전트 빌드 필요 시 실행
go build -o bin/agent ./agent

:: 4. 환경 변수 초기화 (기본 윈도우 설정으로 복원)
set GOOS=
set GOARCH=
```
* **결과물:** `bin/` 디렉토리에 확장자가 없는 리눅스 실행 파일 `server`가 생성됩니다.

---

### Step 3. Windows CMD에서 SCP 전송 실행
절대 경로로 지정된 프라이빗 키(`h:\ssh-key-2026-06-19.key`)를 활용하여 빌드된 파일과 설정을 우분투 서버로 안전하게 전송합니다.

```cmd
:: 1. 수집 서버 바이너리 업로드
scp -i h:\ssh-key-2026-06-19.key bin\server ubuntu@140.245.64.172:~/go-watchdog/server

:: 2. (선택) 설정 파일 업로드 (웹훅 주소 등 변경 사항이 있을 때만 전송)
scp -i h:\ssh-key-2026-06-19.key server\config.json ubuntu@140.245.64.172:~/go-watchdog/config.json
```

---

### Step 4. 우분투 서버에서 권한 설정 및 기동
업로드된 바이너리에 실행 권한을 부여하고, 백그라운드 스레드에서 백그라운드 데몬으로 영구 구동시킵니다.

1. **우분투 SSH 터미널에서 작업 디렉토리로 이동:**
   ```bash
   cd ~/go-watchdog
   ```
2. **새 바이너리에 실행 권한 부여:**
   ```bash
   chmod +x ./server
   ```
3. **nohup을 이용한 백그라운드 재기동:**
   * 터미널 세션이 끊겨도 프로세스가 종료되지 않고 로그를 지속 기록하도록 설정합니다.
   ```bash
   nohup ./server -config config.json > server.log 2>&1 &
   ```
4. **구동 상태 재검증:**
   ```bash
   ps -ef | grep server
   ```
   * 정상적으로 새로운 프로세스 ID가 부여된 채 실행 중으로 출력되면 배포 작업이 마무리됩니다.

---

## 3. 트러블슈팅 (Troubleshooting)

### Q. `scp: dest open "go-watchdog/server": Failure` 에러가 납니다.
* **원인:** 우분투 서버에 기존 `server` 프로세스가 아직 활성화되어 파일이 잠겨 있기 때문입니다.
* **해결책:** **Step 1**을 참조하여 우분투 서버 SSH 터미널에서 `killall server` 또는 `kill [PID]` 명령을 실행하여 구동 중인 프로세스를 확실히 종료한 후 다시 윈도우 CMD에서 `scp` 명령어를 전송해 주세요.
