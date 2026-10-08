# Minecraft Server Manager (Purpur)

Go 기반 마인크래프트 **Purpur** 서버 관리 웹 플랫폼입니다.

---

## 주요 기능

1. **Purpur 구동기 고정 및 자동 설치 (`Downloader`)**
   - Purpur 공식 REST API 연동으로 정식 버전부터 실험 버전/스냅샷까지 전체 버전 및 최신 빌드 원클릭 다운로드
   - 마인크래프트 `eula.txt` 자동 생성 및 동의 (`eula=true`)
2. **프로세스 라이프사이클 제어 (`ProcessManager`)**
   - 웹 대시보드에서 서버 프로세스 시작(`Start`), 종료(`Stop`), 재시작(`Restart`) 원클릭 제어
   - 종료 시 RCON `/stop` 우선 호출 후 graceful shutdown 보장으로 맵 데이터 유실 방지
   - 자식 프로세스 표준 입출력 및 서버 로그 실시간 버퍼링
3. **플러그인 원클릭 관리 (`PluginManager`)**
   - 웹 UI에서 로컬 `.jar` 파일 드래그 앤 드롭 직접 업로드
   - 외부 URL 입력 시 서버가 직접 `plugins/` 폴더로 다운로드 (BlueMap 등 간편 설치)
   - 설치된 플러그인 목록 확인 및 원클릭 삭제
4. **2단계 역할 기반 보안 인증 (`Auth`)**
   - **일반 권한 (`USER`):** 대시보드(접속자 목록, 플레이어 스킨 아바타), BlueMap 지도 뷰어, 웹-인게임 채팅 전송
   - **관리자 권한 (`ADMIN`):** 프로세스 제어, Purpur 설치기, 플러그인 관리, `server.properties` 편집(자동 백업) 및 RCON 콘솔 커맨드
5. **초경량 단일 바이너리 풋프린트**
   - Go Gin 프레임워크 기반 컴파일 + 정적 웹 프론트엔드 임베딩(`embed.FS`)
   - Node.js 런타임 및 외부 RDBMS 의존성 0 (메모리 점유율 수십 MB 수준)

---

## 디렉터리 구조

```text
.
├── cmd/
│   └── server/
│       └── main.go              # 메인 엔트리포인트 (Graceful Shutdown & 프로세스 정리)
├── internal/
│   ├── config/                  # YAML 설정 로더 (JVM 메모리, 경로 등)
│   ├── handler/                 # REST API 및 BlueMap 프록시 컨트롤러
│   ├── mcservice/
│   │   ├── installer.go         # Purpur REST API 연동 다운로더 & EULA 동의
│   │   ├── process.go           # Java 프로세스 라이프사이클 & 콘솔 로그 버퍼
│   │   ├── plugin.go            # plugins/ 디렉터리 I/O, 업로드 & URL 다운로드
│   │   ├── properties.go        # server.properties 파서 및 원자적 저장/백업
│   │   └── rcon.go              # RCON 클라이언트 (플레이어 목록 & tellraw 채팅)
│   ├── middleware/              # JWT 쿠키 기반 역할 검증 미들웨어
│   └── web/                     # 임베디드 정적 프론트엔드 (HTML/CSS/JS)
├── docs/
│   └── proposal.md              # 프로젝트 아키텍처 및 제안서 문서
├── config.yaml                  # 환경설정 파일
├── config.example.yaml          # 환경설정 예시 템플릿
├── server.properties            # 기본 마인크래프트 설정 파일
├── go.mod
└── go.sum
```

---

## 실행 방법

### 1. 설정 확인 (`config.yaml`)

```yaml
server:
  host: "0.0.0.0"
  port: 8080

security:
  user_password: "minecraft"             # 일반 사용자 접속 암호
  admin_password: "admin_minecraft"       # 관리자 전용 암호
  jwt_secret: "dev-secret-key-change-this"

mc:
  rcon_address: "127.0.0.1:25575"
  rcon_password: "rcon_password"
  properties_path: "./server.properties"
  server_dir: "./"
  jar_name: "purpur.jar"
  java_path: "java"                      # OpenJDK 21 실행 경로
  min_memory: "2G"
  max_memory: "4G"

bluemap:
  url: "http://127.0.0.1:8100"           # BlueMap 웹 뷰어 주소
```

### 2. 빌드 및 실행

```bash
# 단일 실행 바이너리 빌드
go build -o mc_manager cmd/server/main.go

# 서버 실행
./mc_manager -config config.yaml
```

웹 브라우저에서 `http://localhost:8080` 접속 후 관리자 비밀번호(`admin_minecraft`)를 입력하면 대시보드와 관리자 탭이 활성화됩니다.

---

## 단위 테스트 실행

```bash
go test -v ./...
```
