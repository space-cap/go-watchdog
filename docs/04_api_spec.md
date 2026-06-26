# [API 명세서] go-watchdog 수집 및 관제 API

본 문서는 `go-watchdog-agent`와 `go-watchdog-server` 간의 통신 및 외부 시스템 연동을 위한 HTTP REST API 명세서입니다.

---

## 1. 개요

* **기본 엔드포인트:** `http://[Server_IP]:9090`
* **통신 포맷:** JSON (HTTP Content-Type: `application/json`)
* **보안 인증:** 모든 리포팅 요청은 사전에 설정된 보안 토큰 검증이 수행됩니다.

---

## 2. API 목록

### 2.1. 메트릭 데이터 전송 (POST)
에이전트가 로컬 시스템에서 수집한 성능 메트릭을 수집 서버에 업로드하는 API입니다.

* **엔드포인트:** `/api/metrics`
* **HTTP Method:** `POST`
* **인증 헤더:**
  * `X-Agent-Token`: 설정된 보안 API Key 문자열 (예: `watchdog-secret-token`)

#### 요청 페이로드 예시 (Request Body)
```json
{
  "agent_id": "windows-db-server-01",
  "cpu_percent": 12.5,
  "mem_total_gb": 15.68,
  "mem_used_gb": 8.54,
  "mem_percent": 54.4,
  "disks": [
    {
      "path": "C:",
      "total_gb": 220.74,
      "used_gb": 137.36,
      "free_gb": 83.38,
      "percent": 62.2
    },
    {
      "path": "D:",
      "total_gb": 270.44,
      "used_gb": 34.24,
      "free_gb": 236.2,
      "percent": 12.6
    }
  ],
  "timestamp": "2026-06-26T17:22:42+09:00"
}
```

#### 파라미터 설명
| 필드명 | 데이터 타입 | 설명 |
| :--- | :--- | :--- |
| **`agent_id`** | `string` | 감시 대상 장비의 고유 식별 명칭 (중복 등록 불가) |
| **`cpu_percent`** | `double` | 전체 CPU 사용률 (%) |
| **`mem_total_gb`** | `double` | 물리 메모리 전체 공간 (GB) |
| **`mem_used_gb`** | `double` | 현재 물리 메모리 사용 공간 (GB) |
| **`mem_percent`** | `double` | 전체 메모리 사용 비율 (%) |
| **`disks`** | `array` | 각 디스크 드라이브별 사용 명세 리스트 |
| **`disks[].path`** | `string` | 디스크 드라이브 문자열 (예: `C:`, `D:`) |
| **`disks[].total_gb`**| `double` | 해당 드라이브 전체 용량 (GB) |
| **`disks[].used_gb`** | `double` | 해당 드라이브 사용량 (GB) |
| **`disks[].free_gb`** | `double` | 해당 드라이브 남은 용량 (GB) |
| **`disks[].percent`** | `double` | 해당 드라이브 사용 비율 (%) |
| **`timestamp`** | `string` | 에이전트 측 성능 메트릭 수집 시간 (ISO 8601 포맷) |

#### 응답 코드
* **`201 Created`** (성공): 데이터베이스 저장 성공
  ```json
  {
    "status": "success"
  }
  ```
* **`400 Bad Request`** (실패): 입력 파라미터 유효성 검사 실패 (JSON 파싱 오류 혹은 필수 필드 누락)
* **`401 Unauthorized`** (실패): `X-Agent-Token` 헤더 누락 혹은 토큰 불일치
* **`500 Internal Server Error`** (실패): 서버 내부 데이터베이스 쓰기 오류 발생 시

---

### 2.2. 모니터링 전체 상태 목록 조회 (GET)
대시보드가 실시간 데이터 갱신을 위해 5초 주기로 호출하거나, 외부 관제 시스템과 데이터를 연동할 때 사용하는 API입니다.

* **엔드포인트:** `/api/status`
* **HTTP Method:** `GET`
* **인증 요구사항:** 인증 없음 (내부 인트라넷 통신 전제)

#### 응답 예시 (Response Body)
```json
[
  {
    "agent_id": "windows-db-server-01",
    "cpu_percent": 8.13,
    "mem_total_gb": 15.68,
    "mem_used_gb": 14.4,
    "mem_percent": 91.0,
    "disks": [
      {
        "path": "C:",
        "total_gb": 220.74,
        "used_gb": 137.36,
        "free_gb": 83.38,
        "percent": 62.2
      }
    ],
    "timestamp": "2026-06-26T17:22:42.1758889+09:00",
    "status": "ONLINE"
  }
]
```

#### 응답 파라미터 설명
수집된 최종 메트릭 DTO 외에 추가적인 관제 메타데이터 정보가 포함됩니다.

| 필드명 | 데이터 타입 | 설명 |
| :--- | :--- | :--- |
| **`status`** | `string` | **에이전트의 현재 활성 여부 (`ONLINE` / `OFFLINE`)** <br> 수집 서버의 현재 시간 기준, `timestamp` 값이 **30초 초과** 지연된 경우 자동으로 `OFFLINE` 판정을 내립니다. |

#### 응답 코드
* **`200 OK`** (성공): 정상 조회 완료
* **`500 Internal Server Error`** (실패): 데이터베이스 조회 에러
