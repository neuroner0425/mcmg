# 마인크래프트 서버 매니저 제안서

## 1. 개요 (Overview)
- **목적:** 최소 리소스로 구동되는 웹 기반 마인크래프트 서버 관리 시스템 구축
- **핵심 목표:**
  - 초경량 풋프린트: Go 단일 바이너리 배포, Node/RDBMS 제거로 메모리 점유 수십 MB 이내 유지
  - Purpur 구동기 고정: Paper/Pufferfish 기반의 고성능 및 상세 커스텀 지원 구동기 채택
  - 프로세스 라이프사이클 제어: 웹에서 원클릭으로 서버 프로세스 시작/종료/재시작
  - Purpur & 플러그인 자동 설치: 최신/실험적 버전 JAR 자동 다운로드, eula 동의, 플러그인 업로드 및 URL 직접 다운로드
  - 2단계 접근 제어: 일반 사용자(플레이어)와 관리자의 권한 분리

## 2. 시스템 아키텍처 (System Architecture)

```
[ 웹 브라우저 (Client) ]
       │ HTTP / Multipart Form (Vanilla JS + CSS)
       ▼
[ Go Gin Web Server ] (포트: 8080 기본)
  ├── Auth Middleware (User / Admin 세션 분기)
  ├── Static / SPA Handlers (BlueMap iframe 연동)
  ├── Process Manager (Java 프로세스 Lifecycle & 로그 버퍼)
  ├── Purpur Installer (api.purpurmc.org 연동 & eula.txt)
  ├── Plugin Manager (plugins/ 디렉터리 I/O, 파일 업로드 & 다운로드)
  ├── Properties Manager (server.properties I/O & 백업)
  └── RCON Service (TCP 25575)
       │
       ▼
[ Purpur Minecraft Server (자식 Java 프로세스) ]
  ├── RCON Listener (포트: 25575)
  ├── plugins/ (BlueMap 등 플러그인)
  └── BlueMap Server (포트: 8100)
```

## 3. 기능 요구사항 (Functional Requirements)

### 3.1 인증 (Authentication)
- **일반 권한 (User):** 서버 접속 패스워드로 로그인. 대시보드(플레이어 목록, 지도, 웹 채팅) 접근 가능.
- **관리자 권한 (Admin):** 관리자 패스워드로 로그인. 서버 프로세스 제어, Purpur/플러그인 설치, `server.properties` 설정 수정 권한 획득.

### 3.2 프로세스 라이프사이클 관리 (Process Lifecycle)
- 상태 머신: `stopped`, `starting`, `running`, `stopping`
- **시작 (Start):** `java -Xms<RAM> -Xmx<RAM> -jar <purpur.jar> nogui` 실행
- **종료 (Stop):** RCON `/stop` 우선 전송 후 정상 종료 대기(Graceful Timeout). 타임아웃 초과 시 SIGTERM/SIGKILL 안전 종료.
- **재시작 (Restart):** 안전 정지 후 재실행.
- **로그 스트리밍:** 프로세스 stdout/stderr 실시간 버퍼링 및 관리자 콘솔 스트리밍.

### 3.3 Purpur 자동 다운로더 (Installer)
- Purpur 공식 REST API (`https://api.purpurmc.org/v2/purpur`) 연동.
- 정식 릴리즈 및 실험적/스냅샷 버전 포함 전체 버전 목록 조회 지원.
- 특정 버전 및 최신(latest) 빌드 JAR 직접 다운로드.
- `eula.txt` 자동 생성 (`eula=true`).

### 3.4 플러그인 관리 (Plugin Management)
- `plugins/` 폴더 내 `.jar` 파일 목록 조회 및 삭제.
- 웹 UI를 통한 로컬 `.jar` 파일 직접 업로드 (Multipart form).
- 외부 다운로드 URL을 입력하여 서버가 직접 `plugins/` 폴더로 다운로드.

### 3.5 플레이어 모니터링, 채팅 및 BlueMap
- RCON `list` 및 `tellraw` 기반 게임 내 채팅 전송.
- BlueMap 웹 지도 iframe 연동.

---

## 4. 기술 스택 및 선정 사유

| 구분 | 기술 스택 | 선정 사유 |
| :--- | :--- | :--- |
| **Backend** | Go (Gin) | 단일 바이너리 컴파일, 네이티브 최적화, 극도로 낮은 메모리 사용량 |
| **MC 구동기** | Purpur | Paper 대비 높은 커스텀 자유도, Pufferfish 최적화 흡수, 경량 환경에서 쾌적 구동 |
| **프로세스 제어** | Go `os/exec` + `sync.Mutex` | 외부 프로세스 매니저(systemd 등) 의존 없이 웹 서버 단독 프로세스 라이프사이클 보장 |
| **MC 통신** | `gorcon/rcon` | 표준 마인크래프트 RCON 프로토콜 준수 |
| **Frontend** | HTML5, Vanilla JS, CSS | 번들러/Node 런타임 제거, Go `embed.FS`로 정적 파일 바이너리 내장 |

---

## 5. 프로젝트 디렉터리 구조

```text
minecraft_server_manager/
├── cmd/
│   └── server/
│       └── main.go              # 메인 엔트리포인트 (Graceful Shutdown 지원)
├── internal/
│   ├── config/                  # 환경설정 로더
│   ├── handler/                 # HTTP/REST 컨트롤러
│   ├── middleware/              # JWT 인가 미들웨어
│   ├── mcservice/               # RCON, Purpur 다운로더, 프로세스 매니저, 플러그인 관리
│   └── web/                     # 내장 웹 프론트엔드 (embed.FS)
├── server/                      # [격리] 마인크래프트 서버 전용 디렉터리
│   ├── purpur.jar               # 다운로드된 구동기
│   ├── server.properties        # 서버 설정 파일
│   ├── eula.txt                 # EULA 자동 동의
│   ├── plugins/                 # 플러그인 디렉터리
│   └── world/, logs/            # 서버 실행 시 생성 데이터 격리
├── data/                        # [격리] 웹 매니저 런타임 저장소
│   └── backups/                 # server.properties 백업 파일 (.bak)
├── docs/proposal.md
├── config.yaml
├── config.example.yaml
├── go.mod
└── go.sum
```
