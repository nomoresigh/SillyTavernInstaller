package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows/registry"
	"gopkg.in/yaml.v3"

	기초 "golang.org/x/sys/windows"
)

const (
	repoURL          = "https://github.com/SillyTavern/SillyTavern.git"
	defaultBranch    = "release"
	stagingBranch    = "staging"
	defaultBaseDir   = "SillyTavern"
	configFileName   = "config.yaml"
	nodeJSWindowsURL = "https://nodejs.org/dist/v22.15.0/node-v22.15.0-x64.msi"
	gitForWindowsURL = "https://github.com/git-for-windows/git/releases/download/v2.49.0.windows.1/Git-2.49.0-64-bit.exe"
)

var isAdmin bool

func init() {
	if runtime.GOOS == "windows" {
		isAdmin = amIAdmin()
	}
}

func amIAdmin() bool { // golang.org/x/sys/windows 의존성 유지
	var sid *기초.SID
	err := 기초.AllocateAndInitializeSid(
		&기초.SECURITY_NT_AUTHORITY, 2,
		기초.SECURITY_BUILTIN_DOMAIN_RID, 기초.DOMAIN_ALIAS_RID_ADMINS,
		0, 0, 0, 0, 0, 0, &sid)
	if err != nil {
		return false
	}
	defer 기초.FreeSid(sid) // FreeSid 사용
	token := 기초.Token(0)  // Token 타입 사용
	member, err := token.IsMember(sid)
	if err != nil {
		return false
	}
	return member
}

// --- main, printMenu, 기타 유틸리티 함수 (clearScreen, setConsoleTitle, printHeader, getUserChoice, checkDependencies, isCommandAvailable, 설치 관련 함수들)는 이전과 동일하게 유지 ---
// (이전 답변의 함수들을 여기에 그대로 가져오거나 필요한 부분만 유지합니다. 공간 절약을 위해 생략)
func main() {
	setConsoleTitle("SillyTavern Installer & Configurator")
	clearScreen()
	printHeader()

	if runtime.GOOS == "windows" {
		if !isAdmin {
			fmt.Println("--------------------------------------------------------------------")
			fmt.Println("ℹ️ 현재 일반 사용자 권한으로 실행 중입니다.")
			fmt.Println("   Git/Node.js 자동 설치 시 Winget/Chocolatey를 우선 사용합니다.")
			fmt.Println("   시스템 PATH를 영구적으로 수정하거나 config.yaml 파일 수정 시")
			fmt.Println("   오류가 발생할 수 있으므로, 이 프로그램을")
			fmt.Println("   마우스 오른쪽 클릭 > '관리자 권한으로 실행'으로 다시 시작하는 것을 권장합니다.")
			fmt.Println("--------------------------------------------------------------------")
		} else {
			fmt.Println("--------------------------------------------------------------------")
			fmt.Println("ℹ️ 현재 관리자 권한으로 실행 중입니다.")
			fmt.Println("   Git/Node.js 자동 설치 시 직접 다운로드 및 설치를 진행하며,")
			fmt.Println("   시스템 PATH에 영구적으로 경로를 추가하고 config.yaml 파일 수정이 가능합니다.")
			fmt.Println("   (Winget/Chocolatey는 관리자 권한 실행 시 건너뜁니다.)")
			fmt.Println("--------------------------------------------------------------------")
		}
		fmt.Println()
	}

	checkDependencies()

	for {
		printMenu()
		choice := getUserChoice()
		clearScreen()

		switch choice {
		case "1":
			installOrUpdateSillyTavern()
		case "2":
			switchBranch()
		case "3":
			changePortSetting()
		case "4":
			updateWhitelistSetting()
		case "5":
			fmt.Println("\n종료합니다...")
			return
		default:
			fmt.Println("\n잘못된 선택입니다. 다시 시도해주세요.")
		}

		fmt.Println("\n계속하려면 엔터를 누르세요...")
		bufio.NewReader(os.Stdin).ReadString('\n')
		clearScreen()
	}
}

func printMenu() {
	fmt.Println("[ 메뉴 ]")
	fmt.Println("1. 실리태번 설치|업데이트")
	fmt.Println("2. 브랜치 변경 (기본|Staging)")
	fmt.Println("3. 포트(Port) 변경")
	fmt.Println("4. 화이트리스트(Whitelist) 수정")
	fmt.Println("5. 종료")
	fmt.Print("\n선택하세요 (1-5): ")
}

func clearScreen() {
	if runtime.GOOS == "windows" {
		cmd := exec.Command("cmd", "/c", "cls")
		cmd.Stdout = os.Stdout
		cmd.Run()
	} else {
		fmt.Print("\033[H\033[2J")
	}
}

func setConsoleTitle(title string) {
	if runtime.GOOS == "windows" {
		cmd := exec.Command("cmd", "/c", "title", title)
		cmd.Run()
	}
}

func printHeader() {
	fmt.Println("        ======================================        ")
	fmt.Println("           SillyTavern Installer & Configurator      ")
	fmt.Println("        ======================================        ")
	fmt.Println("이 도구는 SillyTavern 설치, 업데이트 및 기본 설정을 돕습니다.")
	if runtime.GOOS == "windows" && !isAdmin {
		fmt.Println("현재 일반 사용자 권한으로 실행 중이므로, Winget 또는 Chocolatey (설치된 경우)를 사용합니다.")
	} else if runtime.GOOS == "windows" && isAdmin {
		fmt.Println("현재 관리자 권한으로 실행 중이므로, 직접 다운로드하여 설치하고 시스템 PATH에 추가합니다.")
	}
	fmt.Println("자동 설치 실패 시 아래 URL을 참고하여 수동으로 설치해주세요.")
	fmt.Printf("Node.js (LTS) 수동 다운로드 URL: %s\n", nodeJSWindowsURL)
	fmt.Printf("Git 수동 다운로드 URL: %s\n", gitForWindowsURL)
	fmt.Println("SillyTavern 기본 브랜치:", defaultBranch, "(안정)", "Staging 브랜치:", stagingBranch, "(최신/테스트)")
	fmt.Println("         ======================================        ")
	fmt.Println()
}

func getUserChoice() string {
	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	return strings.TrimSpace(input)
}
func checkDependencies() {
	fmt.Println("필수 프로그램 (Git, Node.js) 확인 중...")
	gitNewlyInstalled := false
	nodeNewlyInstalled := false
	gitOk := isCommandAvailable("git", "--version")
	if !gitOk {
		fmt.Println("❌ Git이 설치되어 있지 않거나 PATH에 없습니다.")
		fmt.Print("Git 자동 설치를 시도하시겠습니까? (y/n): ")
		if strings.ToLower(strings.TrimSpace(getUserChoice())) == "y" {
			if installGit() {
				fmt.Println("✅ Git 설치 또는 PATH 추가 시도 완료.")
				gitNewlyInstalled = true
				if !isAdmin && runtime.GOOS == "windows" {
					fmt.Println("   PATH 변경사항이 적용되려면 이 프로그램을 재시작하거나 새 터미널을 사용해야 할 수 있습니다.")
				} else if isAdmin && runtime.GOOS == "windows" {
					fmt.Println("   시스템 PATH가 업데이트되었을 수 있습니다. 새 터미널에서 'git --version'으로 확인하세요.")
				}
			} else {
				fmt.Println("❌ Git 자동 설치 또는 PATH 추가에 실패했습니다. 수동으로 설치 및 PATH 설정 후 다시 실행해주세요.")
				waitForExit()
			}
		} else {
			fmt.Println("Git 설치가 필요합니다. 프로그램을 종료합니다.")
			waitForExit()
		}
	} else {
		fmt.Println("✅ Git 확인 완료")
	}

	nodeOk := isCommandAvailable("node", "--version") && isCommandAvailable("npm", "--version")
	if !nodeOk {
		fmt.Println("❌ Node.js 또는 npm이 설치되어 있지 않거나 PATH에 없습니다.")
		fmt.Print("Node.js (LTS) 자동 설치를 시도하시겠습니까? (y/n): ")
		if strings.ToLower(strings.TrimSpace(getUserChoice())) == "y" {
			if installNodeJS() {
				fmt.Println("✅ Node.js 설치 또는 PATH 추가 시도 완료.")
				nodeNewlyInstalled = true
				if !isAdmin && runtime.GOOS == "windows" {
					fmt.Println("   PATH 변경사항이 적용되려면 이 프로그램을 재시작하거나 새 터미널을 사용해야 할 수 있습니다.")
				} else if isAdmin && runtime.GOOS == "windows" {
					fmt.Println("   시스템 PATH가 업데이트되었을 수 있습니다. 새 터미널에서 'node --version' 등으로 확인하세요.")
				}
			} else {
				fmt.Println("❌ Node.js 자동 설치 또는 PATH 추가에 실패했습니다. 수동으로 설치 및 PATH 설정 후 다시 실행해주세요.")
				waitForExit()
			}
		} else {
			fmt.Println("Node.js 설치가 필요합니다. 프로그램을 종료합니다.")
			waitForExit()
		}
	} else {
		fmt.Println("✅ Node.js 및 npm 확인 완료")
	}
	fmt.Println()

	// Git 또는 Node.js가 새로 설치되었다면, PATH 적용을 위해 재시작 안내
	if gitNewlyInstalled || nodeNewlyInstalled {
		fmt.Println("\n--------------------------------------------------------------------")
		if gitNewlyInstalled && nodeNewlyInstalled {
			fmt.Println("‼️ 중요: Git 및 Node.js가 방금 설치되었습니다.")
		} else if gitNewlyInstalled {
			fmt.Println("‼️ 중요: Git이 방금 설치되었습니다.")
		} else if nodeNewlyInstalled {
			fmt.Println("‼️ 중요: Node.js가 방금 설치되었습니다.")
		}
		fmt.Println("   변경된 PATH 환경변수가 적용되려면 이 프로그램을 재시작해야 합니다.")
		fmt.Println("   이 창을 닫고 프로그램을 다시 실행하여 주십시오.")
		fmt.Println("--------------------------------------------------------------------")
		waitForExit() // 사용자에게 알리고 종료
	}
}

func isCommandAvailable(cmd string, args ...string) bool {
	command := exec.Command(cmd, args...)
	return command.Run() == nil
}

func installOrUpdateSillyTavern() {
	baseDir := defaultBaseDir
	fmt.Println("\n[ 실리태번 설치/업데이트 ]")

	var currentBranch string
	stDirExists := true
	if _, err := os.Stat(filepath.Join(baseDir, ".git")); err != nil {
		stDirExists = false
	}

	if stDirExists {
		cb, err := getCurrentGitBranch(baseDir)
		if err == nil {
			currentBranch = cb
			fmt.Printf("현재 %s 디렉토리의 브랜치: %s\n", baseDir, currentBranch)
		} else {
			fmt.Printf("⚠️ %s 디렉토리의 현재 브랜치를 가져오는데 실패했습니다: %v\n", baseDir, err)
		}
	}

	if !stDirExists {
		fmt.Printf("%s 디렉토리에 실리태번을 새로 설치합니다 (기본 브랜치: %s)...\n", baseDir, defaultBranch)
		cloneRepo(baseDir, defaultBranch)
	} else {
		if currentBranch == "" {
			fmt.Printf("⚠️ 현재 브랜치를 알 수 없어 기본 브랜치(%s) 기준으로 업데이트합니다.\n", defaultBranch)
			currentBranch = defaultBranch // fallback
		}
		fmt.Printf("%s 디렉토리의 실리태번을 업데이트합니다 (브랜치: %s)...\n", baseDir, currentBranch)
		updateRepo(baseDir, currentBranch)
	}

	installSillyTavernDependencies(baseDir)
	fmt.Println("\n✅ 설치/업데이트 완료!")
}

func cloneRepo(baseDir, branch string) {
	fmt.Printf("실리태번 저장소를 '%s' 브랜치로 클론 중...\n", branch)
	cmd := exec.Command("git", "clone", "-b", branch, repoURL, baseDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Println("\n❌ 저장소 클론에 실패했습니다:", err)
		waitForExit()
	}
}

func updateRepo(baseDir, branchToUpdate string) {
	fmt.Println("저장소 업데이트 중...")
	originalWd, _ := os.Getwd()
	if err := os.Chdir(baseDir); err != nil {
		fmt.Printf("❌ 디렉토리 변경 실패 (%s): %v\n", baseDir, err)
		return
	}
	defer os.Chdir(originalWd)

	fmt.Println("로컬 변경사항 임시 저장 (git stash push -u)...")
	stashCmd := exec.Command("git", "stash", "push", "-u", "-m", "AutoStash_BeforeUpdate_"+time.Now().Format("20060102150405"))
	stashOutput, _ := stashCmd.CombinedOutput()
	if strings.Contains(string(stashOutput), "No local changes to save") || strings.Contains(string(stashOutput), "No stash entries found") {
		fmt.Println("ℹ️ 임시 저장할 로컬 변경사항이 없습니다.")
	} else if stashCmd.ProcessState != nil && stashCmd.ProcessState.Success() {
		fmt.Println("로컬 변경사항 임시 저장 완료.")
	} else {
		fmt.Printf("⚠️ 로컬 변경사항 임시 저장(stash) 중 문제 발생 가능성. 출력:\n%s\n", string(stashOutput))
	}

	fmt.Println("원격 저장소 정보 가져오기 (git fetch origin)...")
	fetchCmd := exec.Command("git", "fetch", "origin")
	fetchCmd.Stdout = os.Stdout
	fetchCmd.Stderr = os.Stderr
	if err := fetchCmd.Run(); err != nil {
		fmt.Println("\n❌ 원격 저장소 정보 가져오기에 실패했습니다:", err)
	}

	if branchToUpdate == "" {
		fmt.Println("\n❌ 업데이트할 브랜치 정보가 없습니다.")
		return
	}

	fmt.Printf("브랜치 (%s) 를 원격 저장소(origin/%s) 기준으로 업데이트 (git pull origin %s)...\n", branchToUpdate, branchToUpdate, branchToUpdate)
	pullCmd := exec.Command("git", "pull", "origin", branchToUpdate)
	pullCmd.Stdout = os.Stdout
	pullCmd.Stderr = os.Stderr
	if err := pullCmd.Run(); err != nil {
		fmt.Println("\n❌ 저장소 업데이트(pull)에 실패했습니다:", err)
		fmt.Println("ℹ️  만약 로컬 변경사항과 충돌이 발생했다면, 수동으로 해결해야 할 수 있습니다.")
	} else {
		fmt.Println("✅ 저장소 업데이트 완료.")
		tryApplyStash(".")
	}
}

func tryApplyStash(repoPath string) {
	checkStashCmd := exec.Command("git", "-C", repoPath, "stash", "list")
	stashListOutput, err := checkStashCmd.Output()
	if err != nil {
		fmt.Printf("⚠️ stash 목록 확인 실패: %v\n", err)
		return
	}
	if strings.TrimSpace(string(stashListOutput)) == "" {
		fmt.Println("ℹ️ 적용할 로컬 변경사항(stash)이 없습니다.")
		return
	}

	fmt.Println("저장된 로컬 변경사항(stash) 자동 복원 시도 (git stash pop)...")
	cmd := exec.Command("git", "-C", repoPath, "stash", "pop")
	var outBuffer, errBuffer bytes.Buffer
	cmd.Stdout = &outBuffer
	cmd.Stderr = &errBuffer

	err = cmd.Run()
	outputCombined := outBuffer.String() + errBuffer.String()

	if err != nil {
		fmt.Println("\n❌ 자동 복원 실패: 로컬 변경사항을 다시 적용하는 중 문제가 발생했습니다.")
		fmt.Println("Git 메시지:")
		fmt.Println("---------------------------------------------------------")
		fmt.Println(strings.TrimSpace(outputCombined))
		fmt.Println("---------------------------------------------------------")
		fmt.Println("ℹ️ 충돌(Conflict)이 발생했을 수 있습니다. 수동으로 해결해야 합니다.")
	} else {
		fmt.Println("\n✅ 저장된 로컬 변경사항(stash)이 성공적으로 복원/적용되었거나, 적용할 내용이 없었습니다.")
		if !strings.Contains(outputCombined, "Dropped refs") && !strings.Contains(outputCombined, "No stash entries found") {
			// fmt.Println("Git 메시지 (참고):")
			// fmt.Println(strings.TrimSpace(outputCombined))
		}
	}
}

func installSillyTavernDependencies(baseDir string) {
	fmt.Println("\nSillyTavern에 필요한 패키지 설치 중 (npm install)... (시간이 다소 걸릴 수 있습니다)")
	originalWd, _ := os.Getwd()
	if err := os.Chdir(baseDir); err != nil {
		fmt.Printf("❌ 디렉토리 변경 실패 (%s): %v\n", baseDir, err)
		return
	}
	defer os.Chdir(originalWd)

	npmCmd := exec.Command("npm", "install")
	npmCmd.Stdout = os.Stdout
	npmCmd.Stderr = os.Stderr
	if err := npmCmd.Run(); err != nil {
		fmt.Println("\n❌ SillyTavern 패키지 설치(npm install)에 실패했습니다:", err)
	} else {
		fmt.Println("✅ SillyTavern 패키지 설치 완료.")
	}
}

func getCurrentGitBranch(repoPath string) (string, error) {
	cmd := exec.Command("git", "-C", repoPath, "rev-parse", "--abbrev-ref", "HEAD")
	branchBytes, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("현재 브랜치 확인 실패: %w", err)
	}
	currentBranch := strings.TrimSpace(string(branchBytes))
	if currentBranch == "HEAD" {
		return "", fmt.Errorf("현재 Detached HEAD 상태입니다.")
	}
	if currentBranch == "" {
		return "", fmt.Errorf("현재 브랜치를 확인할 수 없음")
	}
	return currentBranch, nil
}

func switchBranch() {
	baseDir := defaultBaseDir
	if _, err := os.Stat(filepath.Join(baseDir, ".git")); os.IsNotExist(err) {
		fmt.Println("\n❌ 실리태번이 설치되어 있지 않거나 Git 저장소가 아닙니다. 먼저 설치해주세요.")
		return
	}

	fmt.Println("\n[ 브랜치 변경 ]")
	fmt.Printf("1. 기본 브랜치 (%s)\n", defaultBranch)
	fmt.Printf("2. Staging 브랜치 (%s)\n", stagingBranch)
	fmt.Print("\n선택하세요 (1-2): ")

	choice := getUserChoice()
	var targetBranch string
	switch choice {
	case "1":
		targetBranch = defaultBranch
	case "2":
		targetBranch = stagingBranch
	default:
		fmt.Println("\n잘못된 선택입니다.")
		return
	}

	originalWd, _ := os.Getwd()
	if err := os.Chdir(baseDir); err != nil {
		fmt.Printf("❌ 디렉토리 변경 실패 (%s): %v\n", baseDir, err)
		return
	}
	defer os.Chdir(originalWd)

	currentBranch, err := getCurrentGitBranch(".")
	if err != nil {
		fmt.Printf("\n⚠️ 현재 브랜치를 확인하는데 실패했습니다: %v\n", err)
	}

	if currentBranch == targetBranch {
		fmt.Printf("\n이미 %s 브랜치를 사용 중입니다. 최신 버전으로 업데이트를 시도합니다...\n", targetBranch)
		updateRepo(".", targetBranch)
		installSillyTavernDependencies(".")
		return
	}

	fmt.Printf("\n%s 브랜치로 전환 중...\n", targetBranch)
	fmt.Println("브랜치 전환 전 로컬 변경사항 임시 저장 (git stash push -u)...")
	stashCmd := exec.Command("git", "stash", "push", "-u", "-m", "AutoStash_BeforeBranchSwitch_"+time.Now().Format("20060102150405"))
	stashOutput, _ := stashCmd.CombinedOutput()
	stashedSomething := !strings.Contains(string(stashOutput), "No local changes to save") && !strings.Contains(string(stashOutput), "No stash entries found")
	if stashedSomething {
		fmt.Println("로컬 변경사항 임시 저장 완료.")
	} else {
		fmt.Println("ℹ️ 임시 저장할 로컬 변경사항이 없었습니다.")
	}

	fmt.Printf("원격 저장소에서 %s 브랜치 정보 가져오기 (git fetch origin %s)...\n", targetBranch, targetBranch)
	fetchBranchCmd := exec.Command("git", "fetch", "origin", targetBranch+":"+targetBranch)
	if err := fetchBranchCmd.Run(); err != nil {
		fmt.Printf("\n⚠️ 원격 브랜치(%s)를 가져오는데 실패했을 수 있습니다: %v\n", targetBranch, err)
	}

	fmt.Printf("브랜치 전환 (git checkout %s)...\n", targetBranch)
	checkoutCmd := exec.Command("git", "checkout", targetBranch)
	var checkoutStdErr bytes.Buffer
	checkoutCmd.Stderr = &checkoutStdErr
	checkoutCmd.Stdout = os.Stdout

	if err := checkoutCmd.Run(); err != nil {
		fmt.Println("\n❌ 브랜치 전환에 실패했습니다:", err)
		fmt.Printf("Git 오류: %s\n", checkoutStdErr.String())
		if stashedSomething {
			fmt.Println("ℹ️  'git stash pop'으로 임시 저장된 변경사항을 현재 브랜치에 복원 시도해볼 수 있습니다.")
		}
	} else {
		fmt.Printf("\n✅ %s 브랜치로 전환 완료!\n", targetBranch)
		fmt.Println("전환된 브랜치 최신화 (git pull origin)...")
		updateRepo(".", targetBranch)
		if stashedSomething {
			fmt.Println("\n이전 브랜치에서 가져온 로컬 변경사항 자동 복원 시도...")
			tryApplyStash(".")
		}
		installSillyTavernDependencies(".")
	}
}

func downloadFile(url, targetFilepath string) error {
	fmt.Printf("다운로드 시작: %s -> %s\n", url, targetFilepath)
	dir := filepath.Dir(targetFilepath)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("임시 디렉토리(%s) 생성 실패: %w", dir, err)
		}
	}

	client := http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("stopped after 10 redirects")
			}
			return nil
		},
		Timeout: 60 * time.Second,
	}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("HTTP GET 실패 (%s): %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("잘못된 응답 상태코드 (%s): %s. 응답: %s", url, resp.Status, string(bodyBytes))
	}

	out, err := os.Create(targetFilepath)
	if err != nil {
		return fmt.Errorf("파일 생성 실패 (%s): %w", targetFilepath, err)
	}
	defer out.Close()

	fmt.Println("다운로드 중...")
	size, err := io.Copy(out, resp.Body)
	if err != nil {
		os.Remove(targetFilepath)
		return fmt.Errorf("파일 내용 복사 실패 (%s): %w", targetFilepath, err)
	}
	fmt.Printf("다운로드 완료: %s (%.2f MB)\n", filepath.Base(targetFilepath), float64(size)/(1024*1024))
	return nil
}

func waitForExit() {
	fmt.Println("\n오류가 발생하여 프로그램을 계속 진행할 수 없습니다.")
	fmt.Println("자세한 오류 메시지는 위 내용을 참고하세요.")
	fmt.Println("종료하려면 엔터를 누르세요...")
	bufio.NewReader(os.Stdin).ReadString('\n')
	os.Exit(1)
}

func installProgram(name, wingetID, chocoID, downloadURL, installerName, installerArgs string, versionCheckCmds [][]string) bool {
	fmt.Printf("\n--- %s 자동 설치 시도 ---\n", name)
	installedSuccessfully := false
	attemptedInstall := false

	if runtime.GOOS == "windows" && !isAdmin {
		if wingetID != "" && isCommandAvailable("winget", "--version") {
			attemptedInstall = true
			fmt.Printf("Winget으로 %s 설치 중... (winget install -e --id %s)\n", name, wingetID)
			wingetCmd := exec.Command("winget", "install", "--id", wingetID, "-e", "--accept-source-agreements", "--accept-package-agreements")
			var wgOut, wgErr bytes.Buffer
			wingetCmd.Stdout = &wgOut
			wingetCmd.Stderr = &wgErr
			if err := wingetCmd.Run(); err == nil {
				fmt.Printf("✅ Winget: %s 설치 명령 성공.\n", name)
				// fmt.Println(strings.TrimSpace(wgOut.String())) // 너무 길 수 있음
				installedSuccessfully = true
			} else {
				fmt.Printf("❌ Winget: %s 설치 실패: %v\n", name, err)
				// fmt.Printf("   Winget Stdout: %s\n", wgOut.String()) // 너무 길 수 있음
				// fmt.Printf("   Winget Stderr: %s\n", wgErr.String()) // 너무 길 수 있음
			}
		} else if chocoID != "" && isCommandAvailable("choco", "--version") && !installedSuccessfully {
			attemptedInstall = true
			fmt.Printf("Chocolatey로 %s 설치 중... (choco install %s -y)\n", name, chocoID)
			chocoCmd := exec.Command("cmd", "/c", "choco", "install", chocoID, "-y")
			var chocoOut, chocoErr bytes.Buffer
			chocoCmd.Stdout = &chocoOut
			chocoCmd.Stderr = &chocoErr
			if err := chocoCmd.Run(); err == nil {
				fmt.Printf("✅ Chocolatey: %s 설치 명령 성공.\n", name)
				installedSuccessfully = true
			} else {
				fmt.Printf("❌ Chocolatey: %s 설치 실패: %v\n", name, err)
			}
		}
	}

	if (runtime.GOOS == "windows" && isAdmin) || (runtime.GOOS == "windows" && !isAdmin && !installedSuccessfully) || (runtime.GOOS != "windows") {
		if downloadURL != "" && installerName != "" {
			attemptedInstall = true
			fmt.Printf("직접 다운로드를 통해 %s 설치 시도: %s\n", name, downloadURL)
			tempDir := os.TempDir()
			installerPath := filepath.Join(tempDir, installerName)

			if err := downloadFile(downloadURL, installerPath); err != nil {
				fmt.Printf("❌ %s 다운로드 실패: %v\n", name, err)
				os.Remove(installerPath)
				return false
			}

			fmt.Printf("%s 설치 프로그램 실행 중... (UAC 프롬프트가 나타날 수 있습니다)\n", name)
			var installCmd *exec.Cmd
			if runtime.GOOS == "windows" {
				if strings.HasSuffix(strings.ToLower(installerName), ".msi") {
					installCmd = exec.Command("msiexec", "/i", installerPath, "/quiet", "/norestart", "/L*v", filepath.Join(tempDir, name+"_install.log"))
				} else {
					if name == "Git" && !strings.Contains(installerArgs, "/PATHOPT=") {
						installerArgs += " /PATHOPT=CmdTools"
					}
					if name == "Git" && !strings.Contains(installerArgs, "/COMPONENTS=") {
						installerArgs += " /COMPONENTS=\"icons,ext\\reg\\shellhere,assoc,assoc_sh,gitlfs,scalar\""
					}
					args := strings.Fields(installerArgs)
					fullArgs := append([]string{installerPath}, args...)
					installCmd = exec.Command(fullArgs[0], fullArgs[1:]...)
				}
			} else {
				fmt.Printf("Linux/macOS용 %s 설치 로직은 여기에 구현해야 합니다.\n", name)
				os.Remove(installerPath)
				return false
			}

			var instOut, instErr bytes.Buffer
			installCmd.Stdout = &instOut
			installCmd.Stderr = &instErr
			if err := installCmd.Run(); err != nil {
				fmt.Printf("❌ %s 설치 프로그램 실행 실패: %v\n", name, err)
				fmt.Printf("   Installer Stdout: %s\n", instOut.String())
				fmt.Printf("   Installer Stderr: %s\n", instErr.String())
				if strings.HasSuffix(strings.ToLower(installerName), ".msi") {
					fmt.Printf("   MSI 로그 파일: %s\n", filepath.Join(tempDir, name+"_install.log"))
				}
			} else {
				fmt.Printf("✅ %s 직접 설치 명령 성공.\n", name)
				installedSuccessfully = true
			}
			os.Remove(installerPath)
		} else if downloadURL == "" && installerName == "" && runtime.GOOS == "windows" && isAdmin {
			fmt.Printf("ℹ️ %s: 관리자 권한 실행 중이며, 직접 다운로드 정보가 없어 설치를 건너뜁니다.\n", name)
		}
	}

	if attemptedInstall && len(versionCheckCmds) > 0 {
		fmt.Printf("\n설치된 %s 버전 확인 시도...\n", name)
		allCmdsOk := true
		for _, vc := range versionCheckCmds {
			if !isCommandAvailable(vc[0], vc[1]) {
				allCmdsOk = false
				fmt.Printf("   ❌ '%s %s' 명령어 즉시 사용 불가 (PATH 문제일 수 있음)\n", vc[0], vc[1])
			} else {
				fmt.Printf("   ✅ '%s %s' 명령어 사용 가능\n", vc[0], vc[1])
			}
		}
		if allCmdsOk {
			return true
		}
	} else if !attemptedInstall && len(versionCheckCmds) > 0 {
		allCmdsOk := true
		for _, vc := range versionCheckCmds {
			if !isCommandAvailable(vc[0], vc[1]) {
				allCmdsOk = false
				break
			}
		}
		return allCmdsOk
	}

	return installedSuccessfully
}

func installGit() bool {
	installed := installProgram("Git", "Git.Git", "git.install", gitForWindowsURL, "git_installer.exe", "/VERYSILENT /NORESTART /NOCANCEL /SP- /CLOSEAPPLICATIONS /RESTARTAPPLICATIONS /MERGETASKS=!desktopicon", [][]string{{"git", "--version"}})
	if isCommandAvailable("git", "--version") {
		return true
	}
	if installed {
		fmt.Println("Git이 설치되었을 수 있으나 PATH에서 찾을 수 없습니다.")
		if runtime.GOOS == "windows" && isAdmin {
			fmt.Println("시스템 PATH에 Git 경로 영구 등록을 시도합니다...")
			if addProgramToPathPermanent("Git", []string{
				filepath.Join(os.Getenv("ProgramFiles"), "Git", "cmd"),
				filepath.Join(os.Getenv("ProgramFiles"), "Git", "bin"),
				filepath.Join(os.Getenv("ProgramFiles(x86)"), "Git", "cmd"),
				"C:\\Git\\cmd",
			}) {
				return true
			}
		} else if runtime.GOOS == "windows" && !isAdmin {
			fmt.Println("   PATH를 추가하려면 관리자 권한으로 이 프로그램을 다시 실행해야 합니다.")
		}
	}
	return false
}

func installNodeJS() bool {
	installed := installProgram("Node.js LTS", "OpenJS.NodeJS.LTS", "nodejs-lts", nodeJSWindowsURL, "nodejs_lts_installer.msi", "", [][]string{{"node", "--version"}, {"npm", "--version"}})
	if isCommandAvailable("node", "--version") && isCommandAvailable("npm", "--version") {
		return true
	}
	if installed {
		fmt.Println("Node.js(npm)가 설치되었을 수 있으나 PATH에서 찾을 수 없습니다.")
		if runtime.GOOS == "windows" && isAdmin {
			fmt.Println("시스템 PATH에 Node.js 경로 영구 등록을 시도합니다...")
			if addProgramToPathPermanent("Node.js", []string{
				filepath.Join(os.Getenv("ProgramFiles"), "nodejs"),
				filepath.Join(os.Getenv("ProgramFiles(x86)"), "nodejs"),
			}) {
				return true
			}
		} else if runtime.GOOS == "windows" && !isAdmin {
			fmt.Println("   PATH를 추가하려면 관리자 권한으로 이 프로그램을 다시 실행해야 합니다.")
		}
	}
	return false
}

func addProgramToPathPermanent(programName string, commonPaths []string) bool {
	if runtime.GOOS != "windows" {
		return false
	}
	if !isAdmin {
		return false
	}

	fmt.Printf("시스템 PATH에 %s 설치 경로 추가 시도 중...\n", programName)
	pathSuccessfullyAdded := false
	for _, progPath := range commonPaths {
		if fi, err := os.Stat(progPath); err == nil && fi.IsDir() {
			fmt.Printf("   발견된 %s 관련 경로: %s\n", programName, progPath)
			err := addToSystemPathRegistry(progPath)
			if err != nil {
				fmt.Printf("   ⚠️ '%s' 경로를 시스템 PATH에 추가하는데 실패했습니다: %v\n", progPath, err)
			} else {
				pathSuccessfullyAdded = true
			}
		}
	}
	return pathSuccessfullyAdded
}

// --- YAML 설정 관련 함수 (이전과 동일) ---
func getConfigPath() (string, error) {
	if _, err := os.Stat(defaultBaseDir); os.IsNotExist(err) {
		return "", fmt.Errorf("SillyTavern 디렉토리(%s)를 찾을 수 없습니다. 먼저 설치해주세요.", defaultBaseDir)
	}
	return filepath.Join(defaultBaseDir, configFileName), nil
}

func loadConfig(filePath string) (map[string]interface{}, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("'%s' 파일 읽기 실패: %w", filePath, err)
	}

	var config map[string]interface{}
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, fmt.Errorf("'%s' YAML 파싱 실패: %w", filePath, err)
	}
	return config, nil
}

func saveConfig(filePath string, config map[string]interface{}) error {
	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("YAML 마샬링 실패: %w", err)
	}
	backupPath := filePath + ".bak." + time.Now().Format("20060102150405") // 타임스탬프 백업
	if _, err := os.Stat(filePath); err == nil {
		if errBak := os.Rename(filePath, backupPath); errBak == nil {
			fmt.Printf("기존 설정 파일 '%s'을 '%s'로 백업했습니다.\n", filePath, backupPath)
		} else {
			fmt.Printf("⚠️ 기존 설정 파일 백업 실패: %v\n", errBak)
		}
	}
	err = os.WriteFile(filePath, data, 0644)
	if err != nil {
		if _, bakErr := os.Stat(backupPath); bakErr == nil {
			os.Rename(backupPath, filePath) // 실패 시 백업 복원 시도
			fmt.Printf("⚠️ 파일 쓰기 실패로 백업 '%s'을 복원 시도했습니다.\n", backupPath)
		}
		return fmt.Errorf("'%s' 파일 쓰기 실패: %w", filePath, err)
	}
	return nil
}

func changePortSetting() {
	fmt.Println("\n[ 포트 변경 ]")
	configPath, err := getConfigPath()
	if err != nil {
		fmt.Println("오류:", err)
		return
	}

	config, err := loadConfig(configPath)
	if err != nil {
		fmt.Println("설정 파일 로드 오류:", err)
		return
	}

	currentPortVal, exists := config["port"]
	if exists {
		switch v := currentPortVal.(type) {
		case int:
			fmt.Printf("현재 설정된 포트: %d\n", v)
		case float64: // YAML 라이브러리가 숫자를 float64로 읽을 수도 있음
			fmt.Printf("현재 설정된 포트: %.0f\n", v)
		case string:
			fmt.Printf("현재 설정된 포트: %s\n", v)
		default:
			fmt.Println("현재 포트 정보를 읽을 수 없거나 알 수 없는 형식입니다.")
		}
	} else {
		fmt.Println("현재 설정된 포트 정보가 없습니다. 기본값(예: 8000)으로 간주됩니다.")
	}

	fmt.Print("새로운 포트 번호를 입력하세요 (예: 8000, 1-65535, 비워두면 변경 안 함): ")
	inputPortStr := getUserChoice()
	if strings.TrimSpace(inputPortStr) == "" {
		fmt.Println("입력이 없어 포트를 변경하지 않습니다.")
		return
	}

	newPort, err := strconv.Atoi(inputPortStr)
	if err != nil || newPort < 1 || newPort > 65535 {
		fmt.Println("잘못된 포트 번호입니다. 1에서 65535 사이의 숫자를 입력해주세요.")
		return
	}

	config["port"] = newPort // YAML 라이브러리가 숫자를 적절히 처리

	err = saveConfig(configPath, config)
	if err != nil {
		fmt.Println("❌ 설정 파일 저장 오류:", err)
	} else {
		fmt.Printf("✅ 포트가 %d로 변경되었습니다. SillyTavern을 재시작해야 적용됩니다.\n", newPort)
	}
}

func updateWhitelistSetting() {
	fmt.Println("\n[ 화이트리스트 수정 ]")
	configPath, err := getConfigPath()
	if err != nil {
		fmt.Println("오류:", err)
		return
	}

	config, err := loadConfig(configPath)
	if err != nil {
		fmt.Println("설정 파일 로드 오류:", err)
		return
	}

	// 현재 화이트리스트 로드
	currentWhitelistSet := make(map[string]bool) // 중복 방지를 위해 set처럼 사용
	currentWhitelistDisplay := []string{}        // 화면 표시용

	if wlNode, ok := config["whitelist"]; ok {
		if wlSlice, okSlice := wlNode.([]interface{}); okSlice {
			for _, item := range wlSlice {
				if ipStr, okStr := item.(string); okStr {
					trimmedIP := strings.TrimSpace(ipStr)
					if trimmedIP != "" && !currentWhitelistSet[trimmedIP] {
						currentWhitelistSet[trimmedIP] = true
						currentWhitelistDisplay = append(currentWhitelistDisplay, trimmedIP)
					}
				}
			}
		}
	}

	if len(currentWhitelistDisplay) > 0 {
		fmt.Println("현재 화이트리스트:", strings.Join(currentWhitelistDisplay, ", "))
	} else {
		fmt.Println("현재 화이트리스트: (설정된 IP 없음)")
	}

	fmt.Print("추가할 화이트리스트 IP 주소를 입력하세요 (쉼표(,)로 구분, 예: 192.168.1.100,10.0.0.5, 비워두면 추가 안 함): ")
	inputIPsStr := getUserChoice()

	if strings.TrimSpace(inputIPsStr) == "" {
		fmt.Println("입력이 없어 화이트리스트에 IP를 추가하지 않습니다.")
		return
	}

	addedIPs := 0
	ipsToAdd := strings.Split(inputIPsStr, ",")
	ipRegex := regexp.MustCompile(`^(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)$`)

	for _, ip := range ipsToAdd {
		trimmedIP := strings.TrimSpace(ip)
		if trimmedIP != "" {
			if !ipRegex.MatchString(trimmedIP) {
				fmt.Printf("⚠️ 잘못된 IP 주소 형식입니다: '%s' (무시됨)\n", trimmedIP)
				continue
			}
			if !currentWhitelistSet[trimmedIP] { // 기존 목록에 없는 IP만 추가
				currentWhitelistSet[trimmedIP] = true
				currentWhitelistDisplay = append(currentWhitelistDisplay, trimmedIP) // 화면 표시용 목록에도 추가
				addedIPs++
			} else {
				fmt.Printf("ℹ️ IP 주소 '%s'는 이미 화이트리스트에 존재합니다.\n", trimmedIP)
			}
		}
	}

	if addedIPs == 0 && len(ipsToAdd) > 0 && strings.TrimSpace(inputIPsStr) != "" {
		// 입력은 있었으나 유효한 새 IP가 없거나 모두 중복된 경우
		fmt.Println("새로 추가된 IP 주소가 없습니다 (유효하지 않거나 이미 존재).")
		// return // 여기서 종료할 수도 있고, whitelistMode 설정을 위해 계속 진행할 수도 있음
	}

	// 최종 화이트리스트 목록 생성 (config 파일에 저장할 형태)
	finalWhitelistForYAML := []interface{}{} // YAML에는 []interface{} 또는 []string
	for ip := range currentWhitelistSet {
		finalWhitelistForYAML = append(finalWhitelistForYAML, ip)
	}
	// 순서 유지를 원한다면 currentWhitelistDisplay를 사용하되, 중복 제거 로직을 위에서 잘 처리해야 함.
	// 여기서는 set을 사용했으므로 순서는 보장되지 않지만 중복은 없음.
	// 만약 순서 유지가 중요하다면, currentWhitelistDisplay를 기준으로 finalWhitelistForYAML을 만들어야 함.
	// 예: finalWhitelistForYAML = make([]interface{}, len(currentWhitelistDisplay))
	//     for i, v := range currentWhitelistDisplay { finalWhitelistForYAML[i] = v }

	config["whitelist"] = finalWhitelistForYAML

	err = saveConfig(configPath, config)
	if err != nil {
		fmt.Println("❌ 설정 파일 저장 오류:", err)
	} else {
		fmt.Println("✅ 화이트리스트가 업데이트되었습니다. SillyTavern을 재시작해야 적용됩니다.")
		if len(finalWhitelistForYAML) > 0 {
			// YAML에 저장된 값을 기준으로 다시 문자열 슬라이스 생성 (화면 표시용)
			displaySlice := make([]string, len(finalWhitelistForYAML))
			for i, v := range finalWhitelistForYAML {
				displaySlice[i] = v.(string) // interface{}를 string으로 타입 단언
			}
			fmt.Println("   최종 화이트리스트:", strings.Join(displaySlice, ", "))
		} else {
			fmt.Println("   (화이트리스트 비워짐)")
		}
	}
}

// --- 레지스트리 및 환경변수 브로드캐스트 함수 (수정됨) ---
const (
	regPathEnv           = `SYSTEM\CurrentControlSet\Control\Session Manager\Environment`
	HWND_BROADCAST       = uintptr(0xFFFF)
	WM_SETTINGCHANGE     = uintptr(0x001A)
	SMTO_ABORTIFHUNG     = uintptr(0x0002) // syscall에서 사용할 상수 값
	ERROR_FILE_NOT_FOUND = syscall.Errno(2)
)

func getSystemPathRegistry() (string, error) {
	if runtime.GOOS != "windows" {
		return os.Getenv("PATH"), nil
	}
	// registry.CURRENT_USER 또는 registry.LOCAL_MACHINE
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, regPathEnv, registry.QUERY_VALUE)
	if err != nil {
		return "", fmt.Errorf("레지스트리 키 열기 실패 (HKLM\\%s): %w", regPathEnv, err)
	}
	defer k.Close()

	s, _, err := k.GetStringValue("Path")
	if err != nil {
		// Path 값이 없는 경우 오류 대신 빈 문자열 반환 고려
		if err == registry.ErrNotExist { // registry.ErrNotExist 사용
			return "", nil
		}
		return "", fmt.Errorf("레지스트리에서 Path 값 읽기 실패: %w", err)
	}
	return s, nil
}

func addToSystemPathRegistry(newPathEntry string) error {
	if runtime.GOOS != "windows" {
		return nil
	}
	if !isAdmin {
		return fmt.Errorf("시스템 PATH를 수정하려면 관리자 권한이 필요합니다")
	}

	currentPath, err := getSystemPathRegistry()
	if err != nil {
		return fmt.Errorf("기존 시스템 PATH 읽기 실패: %w", err)
	}

	paths := strings.Split(currentPath, ";")
	newPathEntry = filepath.Clean(newPathEntry)
	for _, p := range paths {
		if strings.EqualFold(filepath.Clean(p), newPathEntry) {
			fmt.Printf("   ℹ️ 경로 '%s'는 이미 시스템 PATH에 존재합니다.\n", newPathEntry)
			return nil
		}
	}

	var newFullPathString string
	if strings.TrimSpace(currentPath) == "" {
		newFullPathString = newPathEntry
	} else {
		// 중복 세미콜론 방지 및 끝 세미콜론 제거
		trimmedCurrentPath := strings.TrimRight(currentPath, ";")
		newFullPathString = trimmedCurrentPath + ";" + newPathEntry
	}

	k, err := registry.OpenKey(registry.LOCAL_MACHINE, regPathEnv, registry.WRITE)
	if err != nil {
		return fmt.Errorf("레지스트리 쓰기 위해 키 열기 실패 (HKLM\\%s): %w", regPathEnv, err)
	}
	defer k.Close()

	// 시스템 PATH는 보통 REG_EXPAND_SZ 타입
	err = k.SetExpandStringValue("Path", newFullPathString)
	if err != nil {
		// 일부 시스템에서는 REG_SZ 일 수도 있음
		errSz := k.SetStringValue("Path", newFullPathString)
		if errSz != nil {
			return fmt.Errorf("레지스트리에 Path 값 쓰기 실패 (EXPAND_SZ: %v, SZ: %v)", err, errSz)
		}
		fmt.Println("   (참고: Path 값을 REG_SZ 타입으로 설정했습니다.)")
	}

	fmt.Printf("   ✅ 레지스트리의 시스템 PATH에 '%s' 추가 요청됨.\n", newPathEntry)

	// 변경 사항 브로드캐스트
	errBroadcast := broadcastEnvironmentChange()
	if errBroadcast != nil {
		fmt.Printf("   ⚠️ 환경 변수 변경 브로드캐스트 실패: %v\n", errBroadcast)
		fmt.Println("      PATH는 변경되었을 수 있으나, 일부 프로그램이 즉시 인지하지 못할 수 있습니다.")
		fmt.Println("      가장 확실한 방법은 시스템을 재시작하는 것입니다.")
	} else {
		fmt.Println("   ✅ 환경 변수 변경 사항이 시스템에 알려졌습니다.")
	}
	return nil
}

func broadcastEnvironmentChange() error {
	if runtime.GOOS != "windows" {
		return nil
	}
	var user32 = syscall.NewLazyDLL("user32.dll")
	var sendMessageTimeout = user32.NewProc("SendMessageTimeoutW")

	// "Environment" 문자열을 UTF-16으로 변환
	env, err := syscall.UTF16PtrFromString("Environment")
	if err != nil {
		return fmt.Errorf("UTF16PtrFromString 변환 실패: %w", err)
	}

	// SendMessageTimeoutW 호출
	// https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-sendmessagetimeoutw
	ret, _, err := sendMessageTimeout.Call(
		HWND_BROADCAST,
		WM_SETTINGCHANGE,
		0, // wParam (사용 안 함)
		uintptr(unsafe.Pointer(env)),
		SMTO_ABORTIFHUNG,
		5000, // 5초 타임아웃
		0,    // lpdwResult (사용 안 함, 결과를 받으려면 변수 포인터 전달)
	)
	// Call 메소드의 마지막 에러는 시스템 에러가 아닐 수 있음. ret 값으로 성공 여부 판단.
	if ret == 0 { // 일반적으로 실패 시 0 반환 (문서 확인 필요)
		// err가 syscall.Errno 타입일 수 있음
		if err != nil && err.Error() != "The operation completed successfully." { // 성공 시에도 err가 nil이 아닐 수 있음
			return fmt.Errorf("SendMessageTimeoutW 호출 실패 (ret 0): %w", err)
		}
		// 추가적인 에러 확인 로직이 필요할 수 있음
	}
	return nil
}
