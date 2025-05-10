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

	"golang.org/x/sys/windows/registry" // 레지스트리 접근용
	"gopkg.in/yaml.v3"                  // YAML 처리용

	기초 "golang.org/x/sys/windows" // 관리자 권한 확인 등 Windows 특정 API용
)

const (
	repoURL          = "https://github.com/SillyTavern/SillyTavern.git"
	defaultBranch    = "release"
	stagingBranch    = "staging"
	defaultBaseDir   = "SillyTavern"
	configFileName   = "config.yaml"
	nodeJSWindowsURL = "https://nodejs.org/dist/v22.2.0/node-v22.2.0-x64.msi"                                             // LTS 버전은 주기적으로 확인/업데이트 필요
	gitForWindowsURL = "https://github.com/git-for-windows/git/releases/download/v2.45.2.windows.1/Git-2.45.2-64-bit.exe" // 최신 버전 확인/업데이트 필요

	// Windows 레지스트리 및 메시지 관련 상수
	regPathEnv       = `SYSTEM\CurrentControlSet\Control\Session Manager\Environment`
	HWND_BROADCAST   = uintptr(0xFFFF)
	WM_SETTINGCHANGE = uintptr(0x001A)
	SMTO_ABORTIFHUNG = uintptr(0x0002)
)

var (
	isAdmin            bool
	gitExecutablePath  string = "git" // 기본값은 PATH에서 찾도록
	nodeExecutablePath string = "node"
	npmExecutablePath  string = "npm"

	// Windows 기본 설치 경로 (동적으로 업데이트될 수 있음)
	defaultGitCmdPathWindows  string
	defaultNodeExePathWindows string
	defaultNpmCmdPathWindows  string
)

func init() {
	if runtime.GOOS == "windows" {
		isAdmin = amIAdmin()

		// Program Files 경로 설정
		progFiles := os.Getenv("ProgramFiles")
		progFilesX86 := os.Getenv("ProgramFiles(x86)")

		// Git 기본 경로 설정
		defaultGitCmdPathWindows = filepath.Join(progFiles, "Git", "cmd", "git.exe")
		if _, err := os.Stat(defaultGitCmdPathWindows); os.IsNotExist(err) && progFilesX86 != "" {
			altPath := filepath.Join(progFilesX86, "Git", "cmd", "git.exe")
			if _, errAlt := os.Stat(altPath); errAlt == nil {
				defaultGitCmdPathWindows = altPath
			}
		}

		// Node.js 기본 경로 설정
		defaultNodeExePathWindows = filepath.Join(progFiles, "nodejs", "node.exe")
		defaultNpmCmdPathWindows = filepath.Join(progFiles, "nodejs", "npm.cmd")
		if _, err := os.Stat(defaultNodeExePathWindows); os.IsNotExist(err) && progFilesX86 != "" {
			altNodePath := filepath.Join(progFilesX86, "nodejs", "node.exe")
			altNpmPath := filepath.Join(progFilesX86, "nodejs", "npm.cmd")
			if _, errAlt := os.Stat(altNodePath); errAlt == nil {
				defaultNodeExePathWindows = altNodePath
				defaultNpmCmdPathWindows = altNpmPath
			}
		}
	}
}

func amIAdmin() bool {
	var sid *기초.SID
	err := 기초.AllocateAndInitializeSid(
		&기초.SECURITY_NT_AUTHORITY, 2,
		기초.SECURITY_BUILTIN_DOMAIN_RID, 기초.DOMAIN_ALIAS_RID_ADMINS,
		0, 0, 0, 0, 0, 0, &sid)
	if err != nil {
		return false
	}
	defer 기초.FreeSid(sid)
	token := 기초.Token(0)
	member, err := token.IsMember(sid)
	if err != nil {
		return false
	}
	return member
}

func main() {
	setConsoleTitle("SillyTavern Installer & Configurator")
	clearScreen()
	printHeader()

	if runtime.GOOS == "windows" {
		if !isAdmin {
			fmt.Println("--------------------------------------------------------------------")
			fmt.Println("ℹ️ 현재 일반 사용자 권한으로 실행 중입니다.")
			fmt.Println("   Git/Node.js 자동 설치 시 Winget/Chocolatey를 우선 사용합니다.")
			fmt.Println("--------------------------------------------------------------------")
		} else {
			fmt.Println("--------------------------------------------------------------------")
			fmt.Println("ℹ️ 현재 관리자 권한으로 실행 중입니다.")
			fmt.Println("   Git/Node.js 자동 설치 시 직접 다운로드 및 설치를 진행하며,")
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

	if p, err := exec.LookPath("git"); err == nil {
		gitExecutablePath = p
		fmt.Println("✅ Git 확인 완료 (경로:", gitExecutablePath, ")")
	} else {
		fmt.Println("❌ Git이 설치되어 있지 않거나 PATH에 없습니다.")
		fmt.Print("Git 자동 설치를 시도하시겠습니까? (y/n): ")
		if strings.ToLower(strings.TrimSpace(getUserChoice())) == "y" {
			installed, foundPath := installGit()
			if installed {
				gitExecutablePath = foundPath
				fmt.Println("✅ Git 설치 또는 PATH 추가 시도 완료.")
				if gitExecutablePath == "git" {
					fmt.Println("   PATH가 즉시 적용되지 않았을 수 있습니다. 이 세션에서는 기본 경로로 시도합니다.")
					if runtime.GOOS == "windows" {
						if _, errStat := os.Stat(defaultGitCmdPathWindows); errStat == nil {
							gitExecutablePath = defaultGitCmdPathWindows
							fmt.Println("   (Windows 기본 Git 경로 사용:", gitExecutablePath, ")")
						} else {
							fmt.Println("   ⚠️ Windows 기본 Git 경로도 찾을 수 없습니다. 'git' 명령이 실패할 수 있습니다.")
						}
					}
				} else {
					fmt.Println("   (사용할 Git 경로:", gitExecutablePath, ")")
				}
				if isAdmin && runtime.GOOS == "windows" {
					fmt.Println("   시스템 PATH가 업데이트되었을 수 있습니다. 다음 실행부터는 자동으로 인식됩니다.")
				}
			} else {
				fmt.Println("❌ Git 자동 설치 또는 PATH 추가에 실패했습니다. 수동으로 설치 및 PATH 설정 후 다시 실행해주세요.")
				waitForExit()
			}
		} else {
			fmt.Println("Git 설치가 필요합니다. 프로그램을 종료합니다.")
			waitForExit()
		}
	}

	nodeFoundInPath := false
	if p, err := exec.LookPath("node"); err == nil {
		nodeExecutablePath = p
		nodeFoundInPath = true
	}
	npmFoundInPath := false
	if p, err := exec.LookPath("npm"); err == nil {
		npmExecutablePath = p
		npmFoundInPath = true
	}

	if nodeFoundInPath && npmFoundInPath {
		fmt.Println("✅ Node.js 및 npm 확인 완료 (node:", nodeExecutablePath, ", npm:", npmExecutablePath, ")")
	} else {
		if !nodeFoundInPath {
			fmt.Println("❌ Node.js를 찾을 수 없습니다.")
		}
		if !npmFoundInPath {
			fmt.Println("❌ npm을 찾을 수 없습니다.")
		}
		fmt.Print("Node.js (LTS) 자동 설치를 시도하시겠습니까? (y/n): ")
		if strings.ToLower(strings.TrimSpace(getUserChoice())) == "y" {
			installed, foundNodePath, foundNpmPath := installNodeJS()
			if installed {
				nodeExecutablePath = foundNodePath
				npmExecutablePath = foundNpmPath
				fmt.Println("✅ Node.js 설치 또는 PATH 추가 시도 완료.")
				if nodeExecutablePath == "node" {
					fmt.Println("   Node.js PATH가 즉시 적용되지 않았을 수 있습니다. 이 세션에서는 기본 경로로 시도합니다.")
					if runtime.GOOS == "windows" {
						if _, errStat := os.Stat(defaultNodeExePathWindows); errStat == nil {
							nodeExecutablePath = defaultNodeExePathWindows
							fmt.Println("   (Windows 기본 Node 경로 사용:", nodeExecutablePath, ")")
						} else {
							fmt.Println("   ⚠️ Windows 기본 Node 경로도 찾을 수 없습니다. 'node' 명령이 실패할 수 있습니다.")
						}
					}
				} else {
					fmt.Println("   (사용할 Node 경로:", nodeExecutablePath, ")")
				}
				if npmExecutablePath == "npm" {
					fmt.Println("   npm PATH가 즉시 적용되지 않았을 수 있습니다. 이 세션에서는 기본 경로로 시도합니다.")
					if runtime.GOOS == "windows" {
						if _, errStat := os.Stat(defaultNpmCmdPathWindows); errStat == nil {
							npmExecutablePath = defaultNpmCmdPathWindows
							fmt.Println("   (Windows 기본 npm 경로 사용:", npmExecutablePath, ")")
						} else {
							fmt.Println("   ⚠️ Windows 기본 npm 경로도 찾을 수 없습니다. 'npm' 명령이 실패할 수 있습니다.")
						}
					}
				} else {
					fmt.Println("   (사용할 npm 경로:", npmExecutablePath, ")")
				}

				if isAdmin && runtime.GOOS == "windows" {
					fmt.Println("   시스템 PATH가 업데이트되었을 수 있습니다. 다음 실행부터는 자동으로 인식됩니다.")
				}
			} else {
				fmt.Println("❌ Node.js 자동 설치 또는 PATH 추가에 실패했습니다. 수동으로 설치 및 PATH 설정 후 다시 실행해주세요.")
				waitForExit()
			}
		} else {
			fmt.Println("Node.js 설치가 필요합니다. 프로그램을 종료합니다.")
			waitForExit()
		}
	}
	fmt.Println()
}

func isCommandAvailable(cmdKey string, args ...string) bool {
	pathToUse := cmdKey
	switch cmdKey {
	case "git":
		if gitExecutablePath != "git" {
			pathToUse = gitExecutablePath
		}
	case "node":
		if nodeExecutablePath != "node" {
			pathToUse = nodeExecutablePath
		}
	case "npm":
		if npmExecutablePath != "npm" {
			pathToUse = npmExecutablePath
		}
	}
	command := exec.Command(pathToUse, args...)
	return command.Run() == nil
}

// main.go 파일의 다른 함수들과 같은 레벨에 추가합니다.

// getNodeJsDir는 node.exe가 위치한 디렉터리 경로를 반환합니다.
// Windows 환경에서만 의미있는 로직을 포함할 수 있습니다.
func getNodeJsDir() string {
	if runtime.GOOS != "windows" {
		// Windows가 아닌 경우, PATH에 의존하거나 nodeExecutablePath가 이미 절대 경로이길 기대합니다.
		if filepath.IsAbs(nodeExecutablePath) {
			return filepath.Dir(nodeExecutablePath)
		}
		return "" // 또는 특정 OS에 맞는 로직 추가
	}

	// 1. nodeExecutablePath가 절대 경로이고 node.exe로 끝나는 경우
	if filepath.IsAbs(nodeExecutablePath) && strings.HasSuffix(strings.ToLower(nodeExecutablePath), "node.exe") {
		return filepath.Dir(nodeExecutablePath)
	}

	// 2. npmExecutablePath가 절대 경로이고 npm.cmd로 끝나는 경우 (npm과 node는 보통 같은 디렉토리에 있음)
	if filepath.IsAbs(npmExecutablePath) && strings.HasSuffix(strings.ToLower(npmExecutablePath), "npm.cmd") {
		// npm.cmd가 있는 디렉토리에 node.exe도 있다고 가정
		nodeDir := filepath.Dir(npmExecutablePath)
		if _, err := os.Stat(filepath.Join(nodeDir, "node.exe")); err == nil {
			return nodeDir
		}
	}

	// 3. defaultNodeExePathWindows (기본 Program Files 경로) 확인
	// defaultNodeExePathWindows는 init()에서 설정됩니다.
	if defaultNodeExePathWindows != "" {
		if _, err := os.Stat(defaultNodeExePathWindows); err == nil {
			return filepath.Dir(defaultNodeExePathWindows)
		}
	}

	// 4. PATH에서 node.exe를 다시 찾아보기 (최후의 수단)
	if p, err := exec.LookPath("node"); err == nil {
		absP, errLookup := filepath.Abs(p)
		if errLookup == nil {
			// LookPath가 node.exe 자체를 반환하므로 그 디렉토리를 가져옴
			return filepath.Dir(absP)
		}
	}

	fmt.Println("⚠️ Node.js 실행 파일 디렉터리를 자동으로 확정할 수 없습니다. PATH 설정에 문제가 있을 수 있습니다.")
	return "" // 찾지 못한 경우 빈 문자열 반환
}

func installOrUpdateSillyTavern() {
	baseDir := defaultBaseDir
	fmt.Println("\n[ 실리태번 설치/업데이트 ]")
	gitDir := filepath.Join(baseDir, ".git")
	_, errSt := os.Stat(baseDir)
	_, errGit := os.Stat(gitDir)
	stDirExists := !os.IsNotExist(errSt) && !os.IsNotExist(errGit)

	currentBranch := ""
	if stDirExists {
		cb, err := getCurrentGitBranch(baseDir)
		if err == nil {
			currentBranch = cb
			fmt.Printf("현재 %s 디렉토리의 브랜치: %s\n", baseDir, currentBranch)
		} else {
			fmt.Printf("⚠️ %s 디렉토리의 현재 브랜치를 가져오는데 실패했습니다: %v\n", baseDir, err)
		}
	} else {
		fmt.Printf("SillyTavern 디렉토리(%s) 또는 Git 저장소를 찾을 수 없습니다.\n", baseDir)
	}

	if !stDirExists {
		fmt.Printf("%s 디렉토리에 실리태번을 새로 설치합니다 (기본 브랜치: %s)...\n", baseDir, defaultBranch)
		cloneRepo(baseDir, defaultBranch)
	} else {
		if currentBranch == "" {
			fmt.Printf("⚠️ 현재 브랜치를 알 수 없어 기본 브랜치(%s) 기준으로 업데이트를 시도합니다.\n", defaultBranch)
			fmt.Println("   만약 이것이 의도치 않은 동작이라면, 먼저 SillyTavern 폴더의 Git 상태를 확인해주세요.")
			currentBranch = defaultBranch
		}
		fmt.Printf("%s 디렉토리의 실리태번을 업데이트합니다 (브랜치: %s)...\n", baseDir, currentBranch)
		updateRepo(baseDir, currentBranch)
	}
	installSillyTavernDependencies(baseDir)
	fmt.Println("\n✅ 설치/업데이트 완료!")
}

func cloneRepo(baseDir, branch string) {
	fmt.Printf("실리태번 저장소를 '%s' 브랜치로 클론 중 (using: %s)...\n", branch, gitExecutablePath)
	cmd := exec.Command(gitExecutablePath, "clone", "-b", branch, repoURL, baseDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Println("\n❌ 저장소 클론에 실패했습니다:", err)
		if exitErr, ok := err.(*exec.ExitError); ok {
			errMsg := string(exitErr.Stderr)
			if strings.Contains(errMsg, "detected dubious ownership") {
				fmt.Println("\n‼️ Git 소유권 문제 감지됨:")
				fmt.Println("   이 문제를 해결하려면, Git Bash 또는 명령 프롬프트에서 다음 명령을 실행하세요:")
				re := regexp.MustCompile(`safe\.directory ([^\s]+)`)
				matches := re.FindStringSubmatch(errMsg)
				if len(matches) > 1 {
					fmt.Printf("   git config --global --add safe.directory %s\n", strings.TrimSpace(matches[1]))
				} else {
					absPath, _ := filepath.Abs(baseDir)
					fmt.Printf("   git config --global --add safe.directory \"%s\"\n", absPath)
				}
				fmt.Println("\n   위 명령어 실행 후 이 프로그램을 다시 시작해주세요.")
			}
		}
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
	stashCmd := exec.Command(gitExecutablePath, "stash", "push", "-u", "-m", "AutoStash_BeforeUpdate_"+time.Now().Format("20060102150405"))
	stashOutput, stashErr := stashCmd.CombinedOutput()

	if stashErr != nil {
		fmt.Printf("⚠️ 로컬 변경사항 임시 저장(stash) 중 오류 발생: %v\n", stashErr)
		fmt.Printf("   Git Stash 출력:\n%s\n", string(stashOutput))
		if strings.Contains(string(stashOutput), "detected dubious ownership") {
			fmt.Println("\n‼️ Git 소유권 문제 감지됨. `git config --global --add safe.directory ...` 명령을 실행하고 재시도해주세요.")
			return
		}
	} else if strings.Contains(string(stashOutput), "No local changes to save") || strings.Contains(string(stashOutput), "No stash entries found") {
		fmt.Println("ℹ️ 임시 저장할 로컬 변경사항이 없습니다.")
	} else {
		fmt.Println("로컬 변경사항 임시 저장 완료.")
	}

	fmt.Println("원격 저장소 정보 가져오기 (git fetch origin)...")
	fetchCmd := exec.Command(gitExecutablePath, "fetch", "origin")
	var fetchErrBuffer bytes.Buffer
	fetchCmd.Stderr = &fetchErrBuffer
	fetchCmd.Stdout = os.Stdout
	if err := fetchCmd.Run(); err != nil {
		fmt.Println("\n❌ 원격 저장소 정보 가져오기에 실패했습니다:", err)
		errMsg := fetchErrBuffer.String()
		fmt.Printf("   Git Fetch 오류:\n%s\n", errMsg)
		if strings.Contains(errMsg, "detected dubious ownership") {
			fmt.Println("\n‼️ Git 소유권 문제 감지됨. `git config --global --add safe.directory ...` 명령을 실행하고 재시도해주세요.")
			return
		}
	}

	if branchToUpdate == "" {
		fmt.Println("\n❌ 업데이트할 브랜치 정보가 없습니다.")
		return
	}

	fmt.Printf("브랜치 (%s) 를 원격 저장소(origin/%s) 기준으로 업데이트 (git pull origin %s)...\n", branchToUpdate, branchToUpdate, branchToUpdate)
	pullCmd := exec.Command(gitExecutablePath, "pull", "origin", branchToUpdate)
	var pullErrBuffer bytes.Buffer
	pullCmd.Stderr = &pullErrBuffer
	pullCmd.Stdout = os.Stdout
	if err := pullCmd.Run(); err != nil {
		fmt.Println("\n❌ 저장소 업데이트(pull)에 실패했습니다:", err)
		errMsg := pullErrBuffer.String()
		fmt.Printf("   Git Pull 오류:\n%s\n", errMsg)
		if strings.Contains(errMsg, "detected dubious ownership") {
			fmt.Println("\n‼️ Git 소유권 문제 감지됨. `git config --global --add safe.directory ...` 명령을 실행하고 재시도해주세요.")
			return
		}
		fmt.Println("ℹ️  만약 로컬 변경사항과 충돌이 발생했다면, 수동으로 해결해야 할 수 있습니다.")
	} else {
		fmt.Println("✅ 저장소 업데이트 완료.")
		tryApplyStash(".")
	}
}

func tryApplyStash(repoPath string) {
	checkStashCmd := exec.Command(gitExecutablePath, "-C", repoPath, "stash", "list")
	stashListOutput, err := checkStashCmd.Output()
	if err != nil {
		fmt.Printf("⚠️ stash 목록 확인 실패: %v\n", err)
		if exitErr, ok := err.(*exec.ExitError); ok && strings.Contains(string(exitErr.Stderr), "detected dubious ownership") {
			fmt.Println("‼️ Git 소유권 문제로 stash 목록 확인 실패. `git config --global --add safe.directory ...` 실행 필요.")
		}
		return
	}
	if strings.TrimSpace(string(stashListOutput)) == "" {
		fmt.Println("ℹ️ 적용할 로컬 변경사항(stash)이 없습니다.")
		return
	}

	fmt.Println("저장된 로컬 변경사항(stash) 자동 복원 시도 (git stash pop)...")
	cmd := exec.Command(gitExecutablePath, "-C", repoPath, "stash", "pop")
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
		if strings.Contains(outputCombined, "detected dubious ownership") {
			fmt.Println("\n‼️ Git 소유권 문제 감지됨. `git config --global --add safe.directory ...` 명령을 실행하고 재시도해주세요.")
		} else {
			fmt.Println("ℹ️ 충돌(Conflict)이 발생했을 수 있습니다. 수동으로 해결해야 합니다.")
		}
	} else {
		fmt.Println("\n✅ 저장된 로컬 변경사항(stash)이 성공적으로 복원/적용되었거나, 적용할 내용이 없었습니다.")
	}
}

func installSillyTavernDependencies(baseDir string) {
	fmt.Printf("\nSillyTavern에 필요한 패키지 설치 중 (npm install, using: %s)...\n", npmExecutablePath)
	originalWd, _ := os.Getwd()
	if err := os.Chdir(baseDir); err != nil {
		fmt.Printf("❌ 디렉토리 변경 실패 (%s): %v\n", baseDir, err)
		return
	}
	defer os.Chdir(originalWd)

	npmCmd := exec.Command(npmExecutablePath, "install")

	if runtime.GOOS == "windows" { // Windows에서 특히 PATH 문제가 발생하므로 명시적 처리
		nodeDir := getNodeJsDir() // 위에서 추가한 헬퍼 함수 사용

		if nodeDir != "" {
			fmt.Printf("ℹ️ Node.js 디렉토리 확인: %s\n", nodeDir)
			currentEnv := os.Environ() // 현재 환경 변수 가져오기
			newEnv := make([]string, 0, len(currentEnv)+1)
			pathVarSet := false

			for _, envVar := range currentEnv {
				if strings.HasPrefix(strings.ToUpper(envVar), "PATH=") {
					originalPath := envVar[len("PATH="):]
					// 새 Node.js 경로를 기존 PATH의 맨 앞에 추가
					newPathValue := nodeDir + string(os.PathListSeparator) + originalPath
					newEnv = append(newEnv, "PATH="+newPathValue)
					pathVarSet = true
					fmt.Printf("   npm 실행을 위해 PATH에 '%s'를 우선 적용합니다.\n", nodeDir)
				} else {
					newEnv = append(newEnv, envVar)
				}
			}

			if !pathVarSet { // PATH 변수가 아예 없는 경우 (매우 드묾)
				newEnv = append(newEnv, "PATH="+nodeDir)
				fmt.Printf("   npm 실행을 위해 PATH에 '%s'를 설정합니다.\n", nodeDir)
			}
			npmCmd.Env = newEnv // 수정된 환경 변수를 Command 객체에 설정
		} else {
			fmt.Println("⚠️ Node.js 디렉토리를 찾지 못해 npm 실행 시 PATH를 명시적으로 설정하지 못했습니다. 기존 PATH에 의존합니다.")
		}
	}
	npmCmd.Stdout = os.Stdout
	npmCmd.Stderr = os.Stderr

	if err := npmCmd.Run(); err != nil {

		fmt.Println("\n❌ SillyTavern 패키지 설치(npm install)에 실패했습니다:", err)
		fmt.Println("   자세한 오류는 위의 npm 출력 및 다음 경로의 로그 파일을 확인해 보세요:")
		userHomeDir, homeErr := os.UserHomeDir()
		if homeErr == nil {
			fmt.Printf("   (예상 로그 경로: %s)\\AppData\\Local\\npm-cache\\_logs\\\n", userHomeDir)
		} else {
			fmt.Println("   (npm 로그는 보통 사용자 AppData\\Local\\npm-cache\\_logs 폴더에 생성됩니다.)")
		}
	} else {
		fmt.Println("✅ SillyTavern 패키지 설치 완료.")
	}
}

func getCurrentGitBranch(repoPath string) (string, error) {
	cmd := exec.Command(gitExecutablePath, "-C", repoPath, "rev-parse", "--abbrev-ref", "HEAD")
	branchBytes, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			errMsg := string(exitErr.Stderr)
			if strings.Contains(errMsg, "detected dubious ownership") {
				return "", fmt.Errorf("Git 소유권 문제로 현재 브랜치 확인 실패: %w", err)
			}
		}
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
		if strings.Contains(err.Error(), "Git 소유권 문제") {
			fmt.Println("   계속 진행하시겠습니까? (y/n): ")
			if strings.ToLower(strings.TrimSpace(getUserChoice())) != "y" {
				return
			}
		}
	}

	if currentBranch == targetBranch {
		fmt.Printf("\n이미 %s 브랜치를 사용 중입니다. 최신 버전으로 업데이트를 시도합니다...\n", targetBranch)
		updateRepo(".", targetBranch)
		installSillyTavernDependencies(".")
		return
	}

	fmt.Printf("\n%s 브랜치로 전환 중...\n", targetBranch)
	fmt.Println("브랜치 전환 전 로컬 변경사항 임시 저장 (git stash push -u)...")
	stashCmd := exec.Command(gitExecutablePath, "stash", "push", "-u", "-m", "AutoStash_BeforeBranchSwitch_"+time.Now().Format("20060102150405"))
	stashOutput, stashErr := stashCmd.CombinedOutput()
	stashedSomething := false
	if stashErr != nil {
		fmt.Printf("⚠️ 로컬 변경사항 임시 저장(stash) 중 오류 발생: %v\n", stashErr)
		fmt.Printf("   Git Stash 출력:\n%s\n", string(stashOutput))
		if strings.Contains(string(stashOutput), "detected dubious ownership") {
			fmt.Println("\n‼️ Git 소유권 문제 감지됨. 이전 안내를 따라 명령 실행 후 재시도해주세요.")
		}
	} else if !strings.Contains(string(stashOutput), "No local changes to save") && !strings.Contains(string(stashOutput), "No stash entries found") {
		fmt.Println("로컬 변경사항 임시 저장 완료.")
		stashedSomething = true
	} else {
		fmt.Println("ℹ️ 임시 저장할 로컬 변경사항이 없었습니다.")
	}

	fmt.Printf("원격 저장소에서 %s 브랜치 정보 가져오기 (git fetch origin %s)...\n", targetBranch, targetBranch)
	fetchBranchCmd := exec.Command(gitExecutablePath, "fetch", "origin", targetBranch+":"+targetBranch)
	var fetchErrBuffer bytes.Buffer
	fetchBranchCmd.Stderr = &fetchErrBuffer
	if err := fetchBranchCmd.Run(); err != nil {
		errMsg := fetchErrBuffer.String()
		fmt.Printf("\n⚠️ 원격 브랜치(%s)를 가져오는데 실패했을 수 있습니다: %v\n", targetBranch, err)
		fmt.Printf("   Git Fetch 오류:\n%s\n", errMsg)
		if strings.Contains(errMsg, "detected dubious ownership") {
			fmt.Println("\n‼️ Git 소유권 문제 감지됨. 계속 진행 시 문제가 발생할 수 있습니다.")
		}
	}

	fmt.Printf("브랜치 전환 (git checkout %s)...\n", targetBranch)
	checkoutCmd := exec.Command(gitExecutablePath, "checkout", targetBranch)
	var checkoutStdErr bytes.Buffer
	checkoutCmd.Stderr = &checkoutStdErr
	checkoutCmd.Stdout = os.Stdout

	if err := checkoutCmd.Run(); err != nil {
		fmt.Println("\n❌ 브랜치 전환에 실패했습니다:", err)
		errMsg := checkoutStdErr.String()
		fmt.Printf("Git 오류: %s\n", errMsg)
		if strings.Contains(errMsg, "detected dubious ownership") {
			fmt.Println("\n‼️ Git 소유권 문제 감지됨. 명령 실행 후 재시도해주세요.")
		}
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

// --- `installGit` 함수 (이전 답변의 수정된 버전) ---
func installGit() (bool, string) {
	foundGitPath := "git"
	installed := installProgram("Git", "Git.Git", "git.install", gitForWindowsURL, "git_installer.exe", "/VERYSILENT /NORESTART /NOCANCEL /SP- /CLOSEAPPLICATIONS /RESTARTAPPLICATIONS /MERGETASKS=!desktopicon", nil)

	if p, err := exec.LookPath("git"); err == nil {
		foundGitPath = p
		return true, foundGitPath
	}

	if installed {
		if runtime.GOOS == "windows" && isAdmin {
			fmt.Println("시스템 PATH에 Git 경로 영구 등록을 시도합니다...")
			pfGitCmd := filepath.Join(os.Getenv("ProgramFiles"), "Git", "cmd")
			if addProgramToPathPermanent("Git", []string{pfGitCmd}) {
				expectedPath := filepath.Join(pfGitCmd, "git.exe")
				if _, errStat := os.Stat(expectedPath); errStat == nil {
					foundGitPath = expectedPath
				} else if runtime.GOOS == "windows" {
					altPfGitCmd := filepath.Join(os.Getenv("ProgramFiles(x86)"), "Git", "cmd")
					altExpectedPath := filepath.Join(altPfGitCmd, "git.exe")
					if _, errStatAlt := os.Stat(altExpectedPath); errStatAlt == nil {
						foundGitPath = altExpectedPath
					}
				}
			}
		}
		// 설치는 되었으므로 true 반환, foundGitPath는 기본값 "git" 또는 예상 경로
		// 만약 defaultGitCmdPathWindows가 유효하다면 그것을 우선 사용
		if foundGitPath == "git" && runtime.GOOS == "windows" {
			if _, errStat := os.Stat(defaultGitCmdPathWindows); errStat == nil {
				foundGitPath = defaultGitCmdPathWindows
			}
		}
		return true, foundGitPath
	}
	return false, foundGitPath
}

// --- `installNodeJS` 함수 (이전 답변의 수정된 버전) ---
func installNodeJS() (bool, string, string) {
	foundNodePath := "node"
	foundNpmPath := "npm"
	installed := installProgram("Node.js LTS", "OpenJS.NodeJS.LTS", "nodejs-lts", nodeJSWindowsURL, "nodejs_lts_installer.msi", "", nil)

	nodePathOk := false
	npmPathOk := false
	if p, err := exec.LookPath("node"); err == nil {
		foundNodePath = p
		nodePathOk = true
	}
	if p, err := exec.LookPath("npm"); err == nil {
		foundNpmPath = p
		npmPathOk = true
	}
	if nodePathOk && npmPathOk {
		return true, foundNodePath, foundNpmPath
	}

	if installed {
		if runtime.GOOS == "windows" && isAdmin {
			fmt.Println("시스템 PATH에 Node.js 경로 영구 등록을 시도합니다...")
			pfNode := filepath.Join(os.Getenv("ProgramFiles"), "nodejs")
			if addProgramToPathPermanent("Node.js", []string{pfNode}) {
				expectedNodePath := filepath.Join(pfNode, "node.exe")
				expectedNpmPath := filepath.Join(pfNode, "npm.cmd")
				if _, errStat := os.Stat(expectedNodePath); errStat == nil {
					foundNodePath = expectedNodePath
				}
				if _, errStat := os.Stat(expectedNpmPath); errStat == nil {
					foundNpmPath = expectedNpmPath
				}
				if foundNodePath == "node" || foundNpmPath == "npm" && runtime.GOOS == "windows" {
					altPfNode := filepath.Join(os.Getenv("ProgramFiles(x86)"), "nodejs")
					altExpectedNodePath := filepath.Join(altPfNode, "node.exe")
					altExpectedNpmPath := filepath.Join(altPfNode, "npm.cmd")
					if foundNodePath == "node" {
						if _, errStatAlt := os.Stat(altExpectedNodePath); errStatAlt == nil {
							foundNodePath = altExpectedNodePath
						}
					}
					if foundNpmPath == "npm" {
						if _, errStatAlt := os.Stat(altExpectedNpmPath); errStatAlt == nil {
							foundNpmPath = altExpectedNpmPath
						}
					}
				}
			}
		}
		// 설치는 되었으므로 true 반환
		// 기본 경로 사용 시도
		if foundNodePath == "node" && runtime.GOOS == "windows" {
			if _, errStat := os.Stat(defaultNodeExePathWindows); errStat == nil {
				foundNodePath = defaultNodeExePathWindows
			}
		}
		if foundNpmPath == "npm" && runtime.GOOS == "windows" {
			if _, errStat := os.Stat(defaultNpmCmdPathWindows); errStat == nil {
				foundNpmPath = defaultNpmCmdPathWindows
			}
		}
		return true, foundNodePath, foundNpmPath
	}
	return false, foundNodePath, foundNpmPath
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
		Timeout: 120 * time.Second,
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

func installProgram(name, wingetID, chocoID, downloadURL, installerName, installerArgs string, _ [][]string) bool {
	fmt.Printf("\n--- %s 자동 설치 시도 ---\n", name)
	installedSuccessfully := false

	if runtime.GOOS == "windows" && !isAdmin {
		if wingetID != "" && isCommandAvailable("winget", "--version") {
			fmt.Printf("Winget으로 %s 설치 중... (winget install -e --id %s)\n", name, wingetID)
			wingetCmd := exec.Command("winget", "install", "--id", wingetID, "-e", "--accept-source-agreements", "--accept-package-agreements")
			wingetCmd.Stdout = os.Stdout
			wingetCmd.Stderr = os.Stderr
			if err := wingetCmd.Run(); err == nil {
				fmt.Printf("✅ Winget: %s 설치 명령 성공.\n", name)
				installedSuccessfully = true
			} else {
				fmt.Printf("❌ Winget: %s 설치 실패: %v\n", name, err)
			}
		}
		if chocoID != "" && !installedSuccessfully && isCommandAvailable("choco", "--version") {
			fmt.Printf("Chocolatey로 %s 설치 중... (choco install %s -y)\n", name, chocoID)
			chocoCmd := exec.Command("cmd", "/c", "choco", "install", chocoID, "-y")
			chocoCmd.Stdout = os.Stdout
			chocoCmd.Stderr = os.Stderr
			if err := chocoCmd.Run(); err == nil {
				fmt.Printf("✅ Chocolatey: %s 설치 명령 성공.\n", name)
				installedSuccessfully = true
			} else {
				fmt.Printf("❌ Chocolatey: %s 설치 실패: %v\n", name, err)
			}
		}
	}

	if !installedSuccessfully && ((runtime.GOOS == "windows" && isAdmin) || (runtime.GOOS == "windows" && !isAdmin) || (runtime.GOOS != "windows")) {
		if downloadURL != "" && installerName != "" {
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
				msiExecPath := "msiexec"
				if _, err := exec.LookPath("msiexec"); err != nil {
					absMsiExecPath := filepath.Join(os.Getenv("SystemRoot"), "System32", "msiexec.exe")
					if _, statErr := os.Stat(absMsiExecPath); statErr == nil {
						msiExecPath = absMsiExecPath
						fmt.Printf("   (msiexec 절대 경로 사용: %s)\n", msiExecPath)
					} else {
						fmt.Printf("   ⚠️ 경고: msiexec를 PATH 및 기본 시스템 경로에서 찾을 수 없습니다. MSI 설치가 실패할 수 있습니다.\n")
					}
				}

				if strings.HasSuffix(strings.ToLower(installerName), ".msi") {
					installCmd = exec.Command(msiExecPath, "/i", installerPath, "/quiet", "/norestart", "/L*v", filepath.Join(tempDir, name+"_install.log"))
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
	return installedSuccessfully
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
	if pathSuccessfullyAdded {
		fmt.Printf("✅ %s 관련 경로가 시스템 PATH에 추가 요청되었습니다.\n", programName)
		fmt.Println("   변경 사항이 모든 프로그램에 적용되려면 새 터미널을 열거나 로그아웃/재부팅해야 할 수 있습니다.")
	} else {
		fmt.Printf("ℹ️ 유효한 %s 설치 경로를 찾지 못했거나 PATH 추가에 실패했습니다.\n", programName)
	}
	return pathSuccessfullyAdded
}

func getConfigPath() (string, error) {
	sillyTavernDir := filepath.Join(".", defaultBaseDir)
	if _, err := os.Stat(sillyTavernDir); os.IsNotExist(err) {
		if _, err2 := os.Stat(defaultBaseDir); os.IsNotExist(err2) {
			return "", fmt.Errorf("SillyTavern 디렉토리(%s 또는 %s)를 찾을 수 없습니다. 먼저 설치해주세요.", sillyTavernDir, defaultBaseDir)
		}
		sillyTavernDir = defaultBaseDir
	}
	return filepath.Join(sillyTavernDir, configFileName), nil
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
	backupPath := filePath + ".bak." + time.Now().Format("20060102_150405")
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
			if errRollback := os.Rename(backupPath, filePath); errRollback == nil {
				fmt.Printf("파일 쓰기 실패로 백업 '%s'을(를) '%s'(으)로 복원했습니다.\n", backupPath, filePath)
			} else {
				fmt.Printf("⚠️ 파일 쓰기 실패 및 백업 복원도 실패: %v\n", errRollback)
			}
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
		case float64:
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
	config["port"] = newPort
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
	currentWhitelistSet := make(map[string]bool)
	currentWhitelistDisplay := []string{}
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
	fmt.Print("추가할 화이트리스트 IP 주소를 입력하세요 (쉼표(,)로 구분, 비워두면 추가 안 함): ")
	inputIPsStr := getUserChoice()
	if strings.TrimSpace(inputIPsStr) == "" {
		fmt.Println("입력이 없어 화이트리스트에 IP를 추가하지 않습니다.")
		return
	}
	addedIPs := 0
	ipsToAdd := strings.Split(inputIPsStr, ",")
	ipRegex := regexp.MustCompile(`^((25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)$`)
	for _, ip := range ipsToAdd {
		trimmedIP := strings.TrimSpace(ip)
		if trimmedIP != "" {
			if !ipRegex.MatchString(trimmedIP) {
				fmt.Printf("⚠️ 잘못된 IP 주소 형식입니다: '%s' (무시됨)\n", trimmedIP)
				continue
			}
			if !currentWhitelistSet[trimmedIP] {
				currentWhitelistSet[trimmedIP] = true
				addedIPs++
			} else {
				fmt.Printf("ℹ️ IP 주소 '%s'는 이미 화이트리스트에 존재합니다.\n", trimmedIP)
			}
		}
	}
	if addedIPs == 0 && len(ipsToAdd) > 0 && strings.TrimSpace(inputIPsStr) != "" {
		fmt.Println("새로 추가된 IP 주소가 없습니다 (모두 유효하지 않거나 이미 존재).")
	}

	finalWhitelistForYAML := make([]interface{}, 0, len(currentWhitelistSet))
	// 순서 유지를 위해 currentWhitelistDisplay (기존 + 새로 유효하게 추가된 IP) 사용
	// 먼저 기존 목록을 넣고
	for _, ip := range currentWhitelistDisplay {
		finalWhitelistForYAML = append(finalWhitelistForYAML, ip)
	}
	// 그 다음, 새로 추가된 (그리고 currentWhitelistDisplay에 아직 없는) IP들을 추가
	// (currentWhitelistSet을 순회하면 순서가 보장되지 않으므로, ipsToAdd를 다시 순회하며 currentWhitelistSet으로 중복 체크)
	tempAddedSet := make(map[string]bool) // 이미 finalWhitelistForYAML에 추가된 새 IP 추적용
	for _, ip := range currentWhitelistDisplay {
		tempAddedSet[ip] = true
	}

	for _, ip := range ipsToAdd {
		trimmedIP := strings.TrimSpace(ip)
		if trimmedIP != "" && ipRegex.MatchString(trimmedIP) && !tempAddedSet[trimmedIP] {
			finalWhitelistForYAML = append(finalWhitelistForYAML, trimmedIP)
			tempAddedSet[trimmedIP] = true
		}
	}

	config["whitelist"] = finalWhitelistForYAML

	err = saveConfig(configPath, config)
	if err != nil {
		fmt.Println("❌ 설정 파일 저장 오류:", err)
	} else {
		fmt.Println("✅ 화이트리스트가 업데이트되었습니다. SillyTavern을 재시작해야 적용됩니다.")
		if len(finalWhitelistForYAML) > 0 {
			displaySlice := make([]string, len(finalWhitelistForYAML))
			for i, v := range finalWhitelistForYAML {
				displaySlice[i] = v.(string)
			}
			fmt.Println("   최종 화이트리스트:", strings.Join(displaySlice, ", "))
		} else {
			fmt.Println("   (화이트리스트 비워짐)")
		}
	}
}

func getSystemPathRegistry() (string, error) {
	if runtime.GOOS != "windows" {
		return os.Getenv("PATH"), nil
	}
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, regPathEnv, registry.QUERY_VALUE)
	if err != nil {
		return "", fmt.Errorf("레지스트리 키 열기 실패 (HKLM\\%s): %w", regPathEnv, err)
	}
	defer k.Close()
	s, _, err := k.GetStringValue("Path")
	if err != nil {
		if err == registry.ErrNotExist {
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
		return fmt.Errorf("시스템 PATH 수정은 관리자 권한 필요")
	}

	currentPath, err := getSystemPathRegistry()
	if err != nil {
		return fmt.Errorf("기존 시스템 PATH 읽기 실패: %w", err)
	}

	paths := strings.Split(currentPath, ";")
	cleanedNewPathEntry := filepath.Clean(newPathEntry)
	for _, p := range paths {
		if strings.EqualFold(filepath.Clean(p), cleanedNewPathEntry) {
			fmt.Printf("   ℹ️ 경로 '%s'는 이미 시스템 PATH에 존재합니다.\n", cleanedNewPathEntry)
			return nil
		}
	}
	var newFullPathString string
	if strings.TrimSpace(currentPath) == "" {
		newFullPathString = cleanedNewPathEntry
	} else {
		trimmedCurrentPath := strings.TrimRight(currentPath, ";")
		newFullPathString = trimmedCurrentPath + ";" + cleanedNewPathEntry
	}
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, regPathEnv, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("레지스트리 쓰기 위해 키 열기 실패 (HKLM\\%s, 권한 확인): %w", regPathEnv, err)
	}
	defer k.Close()
	err = k.SetExpandStringValue("Path", newFullPathString)
	if err != nil {
		errSz := k.SetStringValue("Path", newFullPathString)
		if errSz != nil {
			return fmt.Errorf("레지스트리 Path 값 쓰기 실패 (EXPAND_SZ: %v, SZ: %v)", err, errSz)
		}
		fmt.Println("   (참고: Path 값을 REG_SZ 타입으로 설정했습니다.)")
	}
	fmt.Printf("   ✅ 레지스트리의 시스템 PATH에 '%s' 추가 요청됨.\n", cleanedNewPathEntry)
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
	user32 := syscall.NewLazyDLL("user32.dll")
	if user32.Load() != nil {
		return fmt.Errorf("user32.dll 로드 실패")
	}
	sendMessageTimeout := user32.NewProc("SendMessageTimeoutW")
	if sendMessageTimeout.Find() != nil {
		return fmt.Errorf("SendMessageTimeoutW 프로시저 찾기 실패")
	}
	envStr, err := syscall.UTF16PtrFromString("Environment")
	if err != nil {
		return fmt.Errorf("UTF16PtrFromString(\"Environment\") 변환 실패: %w", err)
	}
	var result uintptr
	ret, _, callErr := sendMessageTimeout.Call(
		HWND_BROADCAST, WM_SETTINGCHANGE, 0, uintptr(unsafe.Pointer(envStr)),
		SMTO_ABORTIFHUNG, 5000, uintptr(unsafe.Pointer(&result)))
	if ret == 0 {
		if callErr != nil && callErr.Error() != "The operation completed successfully." {
			return fmt.Errorf("SendMessageTimeoutW 호출 시스템 오류 (ret 0): %w", callErr)
		}
		return fmt.Errorf("SendMessageTimeoutW API 호출 실패 (반환값 0)")
	}
	return nil
}
