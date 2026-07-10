# [작업 계획서] 대시보드 관리자 권한 제어 및 민감 정보(URL) 보호 시스템 구축

본 문서는 무분별한 헬스체크 대상 등록 및 삭제를 방지하고, 외부 방문자에게는 실제 타겟 URL 주소를 마스킹하여 시스템 보안을 유지하면서 서비스의 가동 상태(ONLINE/OFFLINE)만 모니터링할 수 있도록 하는 권한 제어 시스템 구축 작업 계획서입니다.

---

## 1. 개요 및 목적
* **배경:** 현재 `go-watchdog` 대시보드는 별도의 로그인이나 세션 검증이 없어, 대시보드 주소를 아는 외부인이 임의로 API를 등록하거나 삭제할 수 있으며 민감한 내부 API URL 주소가 그대로 노출됩니다.
* **목적:** 
  * **관리자(Admin) 권한:** 사전 정의된 고유 보안 토큰(`auth_token`)을 통해 인증된 사용자만 대시보드 등록/삭제 권한 부여 및 실제 URL 상세 조회 허용.
  * **방문자(Viewer) 권한:** 인증 없이 대시보드에 접근 가능하나 읽기 전용 상태만 제공. 보안을 위해 **실제 감시 URL 주소는 숨김(마스킹) 처리**.
  * **초경량 설계:** 데이터베이스 회원 관리 테이블을 추가하지 않고, 기존 `config.json`의 `auth_token`을 활용한 **쿠키/세션 인증 방식**을 도입하여 리소스를 극도로 절약합니다.

---

## 2. 권한 등급별 접근 상세 정의

| 권한 등급 | 대시보드 기능 | API CRUD 허용 | API 상태 조회 데이터 | 웹 UI 표현 제어 |
| :--- | :--- | :--- | :--- | :--- |
| **관리자 (Admin)** | 모든 권한 | 등록/삭제 모두 허용 | 실시간 상태 + **실제 URL 전체 노출** | • `[+ API 등록]` 버튼 노출<br>• 각 카드 내 `삭제(🗑️)` 아이콘 노출<br>• 실제 URL 링크 이동 허용 |
| **방문자 (Viewer)** | 읽기 전용 | 모두 차단 (`401 Unauthorized`) | 실시간 상태 + **URL 마스킹 처리**<br>(예: `https://hooks.slack.com/...` ➡️ `Hidden (Admin Only)`) | • `[+ API 등록]` 버튼 숨김<br>• 각 카드 내 `삭제(🗑️)` 아이콘 숨김<br>• URL 링크 클릭 및 숨김 처리 |

---

## 3. 인증 및 통신 아키텍처 흐름

### 3.1 관리자 세션 획득 흐름 (URL 파라미터 인증)
1. 관리자는 최초에 쿼리 파라미터를 실어 접속합니다: `http://localhost:9090/?token=watchdog-secret-token`
2. Go 백엔드 서버는 `token` 파라미터 값을 확인하고, 기존 `auth_token`과 일치하면 클라이언트 브라우저에 **보안 쿠키 (`session_token`)**를 구우며 대시보드 페이지를 정상 서빙합니다.
3. 이후 관리자가 브라우저를 닫을 때까지 쿠키가 유지되어 관리자 권한을 상실하지 않습니다.

### 3.2 데이터 마스킹 흐름
```text
[브라우저 요청] GET /api/health/status
       |
       v
[Go 백엔드 세션 검증]
       |
       |-- (관리자 쿠키 존재 및 유효함) ➡️ 실제 URL 포함 전체 정보 반환
       |
       +-- (쿠키 없거나 무효함) ➡️ "url": "Hidden (Admin Only)" 마스킹 처리 후 반환
```

---

## 4. 상세 변경 계획 (Proposed Changes)

### 4.1 백엔드 보안 제어 고도화 (`server/handler.go`)

#### 1) 관리자 쿠키 검증 유틸 추가
* 요청(Request)의 쿠키 목록 중 `session_token` 값을 읽어 `auth_token`과 일치하는지 판별하는 헬퍼 함수를 추가합니다.
```go
func (s *Server) isAdmin(r *http.Request) bool {
	cookie, err := r.Cookie("session_token")
	if err != nil {
		return false
	}
	return cookie.Value == s.authToken
}
```

#### 2) 대시보드 루트(`/`) 핸들러 수정
* 사용자가 `/?token=xxx` 형태로 접속을 시도하면 토큰 검증 후 쿠키를 저장하도록 수정합니다.
```go
// ServeDashboard 수정 예시
func (s *Server) ServeDashboard(w http.ResponseWriter, r *http.Request) {
    // 1. 토큰 파라미터가 들어온 경우 쿠키 세팅
    tokenParam := r.URL.Query().Get("token")
    if tokenParam == s.authToken {
        http.SetCookie(w, &http.Cookie{
            Name:     "session_token",
            Value:    s.authToken,
            Path:     "/",
            HttpOnly: true, // 자바스크립트 탈취 방지
            MaxAge:   86400, // 1일 유지
        })
        // 쿼리 파라미터 지우기 위해 깔끔하게 루트 경로로 리다이렉트
        http.Redirect(w, r, "/", http.StatusSeeOther)
        return
    }
    // ... 기존 dashboard.html 파일 서빙
}
```

#### 3) `/api/health/status` 상태 조회 핸들러 수정
* `isAdmin(r)` 여부에 따라 각 타겟의 응답 JSON 객체 내부의 `URL` 필드 노출 여부를 동적으로 필터링합니다.
```go
// HandleGetHealthStatus 수정 예시
if !s.isAdmin(r) {
    status.URL = "Hidden (Admin Only)"
}
```

#### 4) 등록 및 삭제 핸들러 보안 미들웨어 강화
* `POST /api/health/targets` 및 `DELETE /api/health/targets` API가 작동할 때 세션 쿠키를 확인하여 권한이 없으면 즉시 `401 Unauthorized` 에러를 응답하도록 처리합니다.

---

### 4.2 프론트엔드 UI/UX 제어 고도화 (`server/templates/` 하위)

#### 1) `dashboard.html` 수정
* 대시보드 로드 시 관리자 여부를 식별할 수 있도록, HTML 바디 어딘가에 관리자 플래그를 간단한 `data-` 속성이나 전역 변수로 내려주거나 혹은 API 응답의 마스킹 여부에 맞춰 동적으로 렌더링되게 구성합니다.

#### 2) `health.js` 수정
* `/api/health/status` 응답 데이터를 기반으로 가용성 카드를 렌더링할 때:
  * 타겟의 `url` 값이 `"Hidden (Admin Only)"`인 경우:
    * `[+ API 등록]` 버튼을 숨김 처리 (`style.display = 'none'`).
    * 각 가용성 카드 내의 `삭제(🗑️)` 버튼을 생성하지 않거나 보이지 않게 처리.
    * 카드 타이틀 하단의 URL 상세 주소를 텍스트 링크가 아닌 마스킹된 텍스트로 대체하여 링크 이동 방지.
  * `url` 값이 정상적인 주소인 경우:
    * 모든 어드민 관련 조작 UI 활성화 및 링크 생성.

---

## 5. 검증 및 테스트 계획

1. **무인증 (Visitor) 접속 검증:**
   * 브라우저 시크릿 창을 열어 `http://localhost:9090`에 접속합니다.
   * `[+ API 헬스체크 등록]` 버튼이 보이지 않는지 확인합니다.
   * 등록되어 있는 API 모니터링 카드들에 휴지통 버튼이 노출되지 않고, URL이 `Hidden (Admin Only)`로 안전하게 가려져 있는지 검증합니다.
2. **관리자 (Admin) 접속 검증:**
   * 브라우저 주소창에 `http://localhost:9090/?token=watchdog-secret-token`을 입력하고 접속합니다.
   * 화면 상단에 등록 버튼이 활성화되고, 카드 내에 URL 상세 경로 및 휴지통 버튼이 보이며 클릭 삭제가 정상 수행되는지 확인합니다.
3. **REST API 무단 강제 요청 차단 검증:**
   * Postman 또는 curl 툴을 사용해 로그인 세션 없이 `POST` 또는 `DELETE` API 호출을 직접 날려보고, 서버가 `401 Unauthorized`로 단호히 요청을 반려하는지 검증합니다.
