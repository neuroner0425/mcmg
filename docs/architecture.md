# 시스템 아키텍처 (System Architecture)

## 1. 개요
본 시스템은 경량 환경에 최적화된 마인크래프트 Purpur 서버 관리 웹 플랫폼이다.
Go 단일 바이너리 및 경량 내장 프론트엔드(Vanilla HTML/CSS/JS, embed.FS)로 구성되어 RDBMS나 별도 Node 런타임 없이 수십 MB 수준의 최소 풋프린트로 구동된다.

```mermaid
graph TD
    UserClient[웹 브라우저 (일반 유저)] -->|WebSocket /ws (chat)| WSHub
    UserClient -->|HTTP/REST /api/chat, /dashboard| WebServer
    AdminClient[웹 브라우저 (관리자)] -->|WebSocket /ws (logs, chat, metrics, command)| WSHub
    AdminClient -->|HTTP/REST /api/admin/*, /admin/*| WebServer

    subgraph "Go Server Manager Process (:8080)"
        WebServer[Gin Engine & Route Handlers]
        WSHub[WebSocket Hub (gorilla/websocket)]
        AuthMid[JWT Auth Middleware (Role-Based)]
        ChatSvc[Chat Service (100-msg Ring Buffer)]
        ProcMgr[Process Lifecycle Manager]
        MetricsSvc[Metrics Service (60-Sample History)]
        PlayerMgmt[Player Mgmt & Whitelist Service]
        BackupMgr[World Backup & Restore Manager]
        SchemaMgr[Properties & Purpur Config Manager]
        PluginMgr[Plugin Manager & Soft Toggle]
        RconSvc[RCON Client Service]
    end

    WebServer --> AuthMid
    WebServer --> WSHub
    WebServer --> ChatSvc
    WebServer --> ProcMgr
    WebServer --> MetricsSvc
    WebServer --> PlayerMgmt
    WebServer --> BackupMgr
    WebServer --> SchemaMgr
    WebServer --> PluginMgr
    WebServer --> RconSvc

    WSHub --> ProcMgr
    WSHub --> ChatSvc
    WSHub --> MetricsSvc
    WSHub --> RconSvc

    subgraph "Minecraft Server Child Process"
        Purpur[Purpur Server JAR (Java 25)]
        BlueMap[BlueMap Plugin (:8100)]
        RconServer[Minecraft RCON Listener (:25575)]
        McStdout[Stdout Stream]
    end

    ProcMgr -->|os/exec & SIGTERM/SIGKILL| Purpur
    Purpur --> McStdout
    McStdout -->|실시간 스캔 & JLine 필터| ChatSvc
    McStdout -->|로그 훅| WSHub
    RconSvc -->|TCP 25575| RconServer
    PlayerMgmt -->|RCON dispatch| RconSvc
    BackupMgr -->|RCON save-off / flush| RconSvc
    AdminClient -->|iframe Embed| BlueMap
```

---

## 2. 주요 모듈 및 서브시스템

### 2.1 인증 및 세션 관리 (`internal/middleware/auth.go`)
- **JWT 기반 2계층 접근 제어:**
  - `User`: 일반 패스워드 인증. 대시보드 모니터링, BlueMap, 인게임 실시간 채팅 조회/전송 가능.
  - `Admin`: 관리자 전용 패스워드 인증. 프로세스 제어, 시스템 메트릭, 화이트리스트 보안, 월드 백업, 플러그인 토글, 설정 스키마 수정, 서버 콘솔 접근 가능.
- **고유 URL 엔드포인트 통제:**
  - `/dashboard`, `/map`은 공용 접근 허용.
  - `/admin/*` 하위의 모든 페이지와 API 엔드포인트는 백엔드 미들웨어에서 HTTP 403 차단 및 프론트엔드 라우터에서 `/dashboard`로 즉시 리디렉션.

### 2.2 프로세스 라이프사이클 및 콘솔 스트림 (`internal/mcservice/process.go`)
- **상태 머신:** `stopped` $\rightarrow$ `starting` $\rightarrow$ `running` $\rightarrow$ `stopping`
- **다계층 Graceful Shutdown 파이프라인:**
  - 서버 중지 요청 시 RCON `/stop` 비동기 디스패치 및 프로세스 표준 입력(`stdin.Write("stop\n")`)을 병행 전송하여 RCON 미연결 상태에서도 청크 저장 및 안전 종료 보장.
  - 10초 내 미종료 시 SIGTERM, 추가 3초 내 미종료 시 SIGKILL 단계적 강제 회수.
  - 매니저 재시작 시 잔류 고아(Orphaned) 프로세스를 `pgrep`으로 자동 감지하여 `session.lock` 충돌 원천 차단.
- **RCON 자동 보정 (`EnsureRCONConfig`):**
  - 서버 구동 전 `server.properties`의 `enable-rcon=true`, `rcon.port=25575`, `rcon.password`를 자동 검증 및 교정하여 관리 채널 항시 유지.
- **로그 버퍼 및 자동 사전 적재:**
  - 서버 시작 시 `server/logs/latest.log`의 최근 200줄을 인메모리 버퍼로 사전 적재.
  - 비터미널 JLine 환경의 프롬프트 중복(`> > > >`) 및 ANSI 제어 문자 자동 필터링.

### 2.3 시스템 리소스 실시간 메트릭 모니터링 (`internal/mcservice/metrics.go`)
- **독립 어드민 전용 페이지 (`/admin/metrics`):**
  - JVM Java 프로세스의 실시간 CPU 점유율 및 RSS 메모리(MB) 측정.
  - RCON `tps` 정규식 파싱을 통한 1분, 5분, 15분 틱 레이트 추적.
  - 불필요한 디스크 카드를 배제하고 CPU, RAM, TPS 3개 집중 카드로 정렬.
  - 프로세스 런타임 정보 카드에 Process ID (PID), 가동 시간, 플레이어 수 표기.
  - **순수 HTML5 Canvas 2D 기반 실시간 시계열 그래프 (`#metricsChartCanvas`):** CPU(%), 메모리(GB), TPS 트렌드 선 그래프 및 배경 그리드 렌더링.

### 2.4 화이트리스트 보안 통제 및 인게임 플레이어 관리 (`internal/mcservice/player_mgmt.go`)
- **Mojang 정품 온라인 UUID 자동 해석 및 마이그레이션:**
  - `online-mode: true` 환경에서 오프라인 해시 대신 Mojang 공식 프로필 API(`https://api.mojang.com/users/profiles/minecraft/<username>`)를 조회하여 정품 UUID를 발급 및 기록 (오프라인 환경 시 자동 fallback).
  - 기존 `whitelist.json` 내 오프라인 UUID로 잘못 저장된 엔트리를 정품 UUID로 자동 감지 및 무중단 마이그레이션.
- **화이트리스트 통제 (`/admin/whitelist`):**
  - `server.properties`의 `white-list` 및 `enforce-whitelist` 플래그 원클릭 웹 토글.
  - RCON `whitelist on/off/add/remove/reload` 인게임 즉시 동기화.
- **대시보드 인터랙티브 플레이어 관리 모달:**
  - 대시보드 온라인 플레이어 클릭 시 모달 오픈.
  - 개인 메시지 전송 (`tellraw <player>`), 게임모드 변경, 주요 아이템 지급 (`give`), 스폰 이동 (`tp`), 인벤토리 초기화 (`clear`), OP 부여/회수, 추방 (`kick`), 영구 차단 (`ban`).

### 2.5 월드 백업 및 BlueMap 자동 에셋 구성
- **월드 안전 백업 (`internal/mcservice/backup.go`):**
  - 백업 시 RCON `save-off` $\rightarrow$ `save-all flush` 호출 후 `server/world*` 데이터를 `data/backups/*.tar.gz`로 비동기 압축 생성하고 `save-on` 복원.
  - 웹 매니저에서 백업 목록 조회, 브라우저 다운로드, 영구 삭제 지원.
- **서버 다운로드 시 BlueMap 자동 다운로드 파이프라인:**
  - 구동기 인스톨러에서 Purpur 서버 다운로드 완료 시 백그라운드로 BlueMap 3D 지도 플러그인(`BlueMap.jar`)을 자동 연쇄 다운로드.
  - `server/plugins/BlueMap/core.conf`의 `accept-download: true`를 자동 사전 주입(Pre-seed)하여 모장 클라이언트 텍스처 에셋이 무인 상태에서 100% 자동 다운로드되도록 보장.
- **플러그인 소프트 토글 (`internal/mcservice/plugin.go`):**
  - `.jar` 파일을 `.jar.disabled`로 확장자 변경하여 서버 구동기에서 물리 삭제 없이 손쉽게 활성화/비활성화 전환.

### 2.6 1급 워크스페이스 내비게이션 구조 (9개 독립 전용 뷰)
1. **대시보드 (`/dashboard`):** 접속자 모니터링, 관리자 인터랙티브 제어 모달, 인게임 실시간 채팅
2. **BlueMap 지도 (`/map`):** 3D 실시간 월드 렌더링 지도 (HTML5 Fullscreen API 기반 원클릭 전체 화면 모드 지원)
3. **시스템 리소스 (`/admin/metrics` - Admin):** 실시간 CPU/RAM/TPS/디스크 게이지 및 시계열 기록
4. **화이트리스트 (`/admin/whitelist` - Admin):** 접속 제한 보안 스위치, 신규 등록, 등록된 플레이어 목록
5. **서버 상세 설정 (`/admin/config` - Admin):** 1,363개 전수 설정 검색, 카테고리 필터링, 기본값 복원, 일괄 저장
6. **구동기 인스톨러 (`/admin/installer` - Admin):** Purpur 빌드 API 브라우징, 버전 선택, 병렬 다운로드
7. **플러그인 관리 (`/admin/plugins` - Admin):** 광역 드래그 앤 드롭 업로드존, 상태 뱃지, 소프트 토글, 삭제
8. **월드 백업 (`/admin/backups` - Admin):** 즉시 스냅샷 생성, 백업 파일 목록, 다운로드, 삭제
9. **서버 콘솔 (`/admin/console` - Admin):** 터미널 뷰어, 퀵 액션, 대화형 명령어 자동 완성(Autocomplete)

### 2.7 WebSocket 양방향 스트리밍 허브 (`internal/mcservice/ws_hub.go`)
- **실시간 터미널 & 채팅 0ms 스트리밍:**
  - `ProcessManager`와 `ChatService`에 이벤트 리스너 훅을 등록하여 새 로그와 인게임 채팅 발생 즉시 연결된 클라이언트에 브로드캐스트.
  - 관리자 전용 데이터(`logs`, `metrics`, `command_result`)와 전체 사용자 데이터(`chat`)를 역할 기반으로 엄격히 격리 전송.
- **2초 주기 백그라운드 메트릭 티커:**
  - 활성 관리자 클라이언트가 접속 중일 때만 백그라운드 티커를 활성화하여 불필요한 시스템 호출 방지.
- **Heartbeat & 회복력:**
  - 25초 주기 WebSocket Ping 프레임 전송 및 60초 Read Deadline을 통한 고스트 연결 자동 청소.
  - 클라이언트 측 지수 백오프 자동 재연결 및 HTTP API Fallback 이중화.

### 2.8 상단바 월드 환경 컨트롤러 (`internal/mcservice/metrics.go`)
- **실시간 날씨 & 시간 위젯 및 Purpur 1.21+ 호환 파이프라인:**
  - **시각 쿼리 호환성:** Purpur 1.21+ 타임라인 문법에 맞춰 `time query day`를 주 쿼리로 사용하고 정규식(`(?:is\s+at\s+|The time is\s+)(\d+)` 및 `(\d+)\s*tick`)으로 틱(0~24000)을 추출하여 시각(HH:mm) 및 위상(일출/정오/일몰/자정 등)으로 변환.
  - **날씨 상태 추적:** 바닐라 RCON에 부재한 날씨 조회를 보완하기 위해 서버 stdout 로그 스트림(`ProcessManager.appendLog`)에서 `Set the weather to ...` 이벤트를 실시간 파싱하고, 서버 시작 시 `latest.log`를 역방향 스캔하여 초기 날씨(`clear`, `rain`, `thunder`)를 완벽 복원.
  - **동시성 및 양방향 제어:** `sync.RWMutex` 기반의 스레드 안전성 보장 및 웹소켓(2초 주기 브로드캐스트) + HTTP 폴링(2.5초 fallback) 이중화 구조.
  - **인터랙티브 제어 및 낙관적 UI 업데이트:** 마우스 드래그 기반 Range 슬라이더 및 프리셋(낮/정오/일몰/밤), 날씨 원클릭 순환 토글 버튼 제공. 조작 즉시 클라이언트 상태를 선반영(Optimistic Update)한 후 RCON `time set` / `weather`를 디스패치하여 부드러운 조작감 보장.
### 2.9 Purpur 구동기 인스톨러 및 런타임 영속성 (`internal/mcservice/installer.go`)
- **버전 내림차순 정렬 및 최신 릴리스 기본화:**
  - Purpur API의 오름차순(구버전 우선) 버전 배열을 내림차순(최신 버전 우선: `26.3` -> `1.21.4` -> ...)으로 역정렬하여 전달.
  - 최신 릴리스가 항상 최상단 및 기본값으로 설정되어 의도치 않은 구버전 다운로드 방지.
- **실시간 프로그레스 바 스트리밍 버그 픽스:**
  - 기존 프론트엔드 폴링의 필드명 불일치(`in_progress` vs `is_busy`)로 인한 0% 조기 중단 버그 해결.
  - 백엔드 `GetStatus()`에서 스트리밍 청크 단위 다운로드 용량(MB), 총 용량, 전송 속도(MB/s)를 원자적(atomic)으로 계산하여 실시간 제공.
- **설치 버전 메타데이터 영속화 및 자동 유지:**
  - 구동기 다운로드 완료 시 `data/installed_version.json`에 버전, 빌드 번호, 파일 용량, 설치 일시를 영속화.
  - `/api/admin/purpur/installed` 엔드포인트를 통해 관리자 대시보드 로드 시 이전에 설치했던 버전을 기억하고, 버전 선택 셀렉트 박스에서 해당 버전을 기본 선택 상태로 유지.
- **2열 반응형 그리드 레이아웃 및 여백 최적화:**
  - `installerPane`의 기존 640px 너비 제한을 제거하고 `repeat(auto-fit, minmax(360px, 1fr))` 2열 그리드로 개편.
  - 좌측: 구동기 버전/빌드 선택 및 실시간 진행률 카드.
  - 우측: 현재 구동기 버전, 빌드 번호, 파일 용량, EULA 동의 상태, BlueMap 연동 상태를 한눈에 볼 수 있는 런타임 상세 카드 배치.

### 2.10 RCON/채팅 자동 활성화, Stdin Fallback 및 BlueMap 지형·플레이어 렌더링 파이프라인 (`internal/mcservice/process.go`)
- **보안 채팅 및 RCON 자동 강제 적용 (`EnsureServerProperties`):**
  - 초기 기동 및 신규 서버 생성 시 `server.properties`에 `enable-rcon=true`, `rcon.port=25575`, `rcon.password=...`, `white-list=true`, `enforce-whitelist=true`, 그리고 **`enforce-secure-profile=false`**를 사전 주입 및 실시간 강제 보정.
  - 모장 서명 누락으로 인한 인게임 채팅 불가 현상(`Chat disabled due to missing profile public key`)을 영구적으로 방지.
- **다차원 월드 구조 자동 연결 (`EnsureWorldStructure`):**
  - 마인크래프트 26.3의 분기된 차원 디렉터리(`world/dimensions/minecraft/*`)와 루트 `world/level.dat` 사이의 심볼릭 링크 자동 생성.
  - `world/region -> dimensions/minecraft/overworld/region` 링크를 구축하여 BlueMap이 지형 청크를 온전히 인식하고 렌더링할 수 있도록 보장.
- **BlueMap 실시간 플레이어 위치 및 마커 디스크 동기화 (`EnsureBlueMapConfig`):**
  - `plugin.conf`의 `write-players-interval: 1` 및 `write-markers-interval: 2`를 활성화하여, 웹소켓 우회나 역방향 프록시 환경에서도 디스크 파일(`live/players.json`)을 통해 100% 실시간 플레이어 위치 및 헤드 텍스처를 웹 지도에 표시.
- **서버 부팅 시 초기 지형 렌더링 태스크 자동 스케줄링:**
  - 서버 시작 및 RCON 연결 완료 시 `bluemap update overworld`를 1회 자동 디스패치하여 관리자의 수동 조작 없이도 3D 지형 타일 빌드가 시작되도록 설계.
- **표준 입력(Stdin) 이중화 Fallback (`WriteStdin` & `dispatchCommand`):**
  - 일시적 RCON 연결 지연 또는 단절 시, WebSocket 및 플레이어 관리 명령어(화이트리스트 등록, 갱신 등)를 자식 프로세스 표준 입력(`stdin`) 파이프로 즉시 전달하여 명령어가 누락 없이 서버 콘솔에 투입되도록 보장.

---

## 3. 격리된 디렉터리 아키텍처

```text
minecraft_server_manager/
├── cmd/server/main.go          # 서버 엔트리포인트 및 의존성 주입
├── internal/
│   ├── config/                 # YAML 환경설정 파서
│   ├── handler/                # REST 핸들러 (메트릭, 화이트리스트, 백업, 플러그인 등)
│   ├── mcservice/              # 코어 비즈니스 로직 (프로세스, 메트릭, 플레이어관리, 백업, 인스톨러)
│   ├── middleware/             # 역할 기반 JWT 인증 미들웨어
│   └── web/                    # SPA 정적 웹 파일 및 고유 엔드포인트 라우팅 (embed.FS)
├── server/                     # [격리] 마인크래프트 게임 전용 영역
│   ├── purpur.jar              # 서버 구동기
│   ├── installed_version.json  # [영속화] 설치된 Purpur 버전/빌드 메타데이터
│   ├── server.properties       # 표준 서버 설정 (white-list, enforce-whitelist 등)
│   ├── purpur.yml              # Purpur 상세 설정
│   ├── whitelist.json          # 화이트리스트 등록 플레이어 목록
│   ├── plugins/                # 플러그인 (*.jar, *.jar.disabled)
│   ├── world/                  # 월드 데이터
│   └── logs/latest.log         # 마인크래프트 운영 로그
├── data/                       # [격리] 웹 매니저 데이터 영역
│   └── backups/                # 월드 압축 백업(*.tar.gz) 및 설정 자동 백업 저장소
├── logs/                       # 웹 매니저 HTTP 접근 로그 (access.log)
├── docs/                       # 시스템 아키텍처 및 ADR 문서
└── config.yaml                 # 웹 매니저 환경설정
```

