# go-watchdog 문서 저장소

`go-watchdog` 시스템의 기획, 아키텍처 설계, 개발 규격 및 운영 관리를 위한 전체 문서 목록입니다.

---

## 문서 목차

### 1단계: 기획 및 설계
* [01_프로젝트 기획서](file:///h:/lee/go-watchdog/docs/01_기획서.md)  
  시스템 구축 목적, 요구 사양 및 전체 아키텍처 아웃라인을 다룹니다.
* [02_시스템 구현 계획서](file:///h:/lee/go-watchdog/docs/02_implementation_plan.md)  
  구현을 위해 검토된 라이브러리 선정 정보 및 세부 구현 전략을 기술합니다.

### 2단계: 기술 명세
* [03_데이터베이스 명세서](file:///h:/lee/go-watchdog/docs/03_db_spec.md)  
  내장형 SQLite DB 스키마 설계 및 테이블 상세 속성, 성능 최적화 설정을 기록합니다.
* [04_수집 및 관제 API 명세서](file:///h:/lee/go-watchdog/docs/04_api_spec.md)  
  에이전트 수집 정보 리포팅 규격 및 관제 모니터링 연동을 위한 JSON 스키마를 제공합니다.

### 3단계: 배포 및 운영
* [05_방화벽 및 Windows 서비스 배포 가이드](file:///h:/lee/go-watchdog/docs/05_deployment_guide.md)  
  방화벽 규칙 활성화 방법과 NSSM을 통한 에이전트 서비스화 절차를 안내합니다.
* [06_장애 대응 트러블슈팅 가이드](file:///h:/lee/go-watchdog/docs/06_troubleshooting.md)  
  DB 락 현상, 통신 장애, 오프라인 오탐지 등 상황별 점검 요령과 극복 방안을 설명합니다.
* [07_OS별 빌드 가이드](file:///h:/lee/go-watchdog/docs/07_build_guide.md)  
  Windows 및 Ubuntu(Linux) OS에서의 기본 로컬 빌드 및 크로스 컴파일 명령어 세트를 안내합니다.
