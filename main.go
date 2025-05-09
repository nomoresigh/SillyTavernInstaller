package main

import (
	"bufio"
	"bytes" // For capturing command output
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	// 설치 경로 및 저장소 관련 상수
	repoURL       = "https://github.com/SillyTavern/SillyTavern.git"
	defaultBranch = "release"
	stagingBranch = "staging"
	defaultBaseDir = "SillyTavern"

	// 자동 설치 관련 상수 - winget/choco 실패 시 수동 설치용 예시
	nodeJSWindowsURL = "https://nodejs.org/dist/v22.2.0/node-v22.2.0-x64.msi"
	gitForWindowsURL = "https://github.com/git-for-windows/git/releases/download/v2.45.1.windows.1/Git-2.45.1-64-bit.exe"

	// 윈도우용 패키지 매니저
	wingetCheckCommand     = "winget"
	chocolateyCheckCommand = "choco"
)

func main() {
	setConsoleTitle("SillyTavern Installer")
	clearScreen()
	printHeader()
	checkDependencies()

	for {
		printMenu()
		choice := getUserChoice()
		clearScreen() // 메뉴 선택 후 화면 정리

		switch choice {
		case "1":
			installOrUpdateSillyTavern()
		case "2":
			switchBranch()
		case "3":
			runSillyTavern()
		case "4":
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

func clearScreen() {
	cmd := exec.Command("cmd", "/c", "cls")
	cmd.Stdout = os.Stdout
	cmd.Run()
}

func setConsoleTitle(title string) {
	cmd := exec.Command("cmd", "/c", "title", title)
	cmd.Run()
}

func printHeader() {
	fmt.Println("        ======================================        ")
	fmt.Println("                  SillyTavern Installer      ")
	fmt.Println("        ======================================        ")
	fmt.Println("이 도구는 Git과 Node.js (LTS)를 자동으로 설치하려고 시도합니다.")
	fmt.Println("winget 또는 Chocolatey가 설치되어 있으면 이를 우선 사용합니다.")
	fmt.Println("자동 설치 실패 시 아래 URL을 참고하여 수동으로 설치해주세요.")
	fmt.Printf("Node.js (LTS) 수동 다운로드 URL: %s\n", nodeJSWindowsURL)
	fmt.Printf("Git 수동 다운로드 URL: %s\n", gitForWindowsURL)
	fmt.Println("SillyTavern 기본 브랜치:", defaultBranch, "(안정)", "Staging 브랜치:", stagingBranch, "(최신/테스트)")
	fmt.Println("         ======================================        ")
	fmt.Println()
}

func printMenu() {
	fmt.Println("[ 메뉴 ]")
	fmt.Println("1. 실리태번 설치|업데이트")
	fmt.Println("2. 브랜치 변경 (기본|Staging)")
	fmt.Println("3. 실리태번 실행")
	fmt.Println("4. 종료")
	fmt.Print("\n선택하세요 (1-4): ")
}

func getUserChoice() string {
	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	return strings.TrimSpace(input)
}

func checkDependencies() {
	fmt.Println("필수 프로그램 (Git, Node.js) 확인 중...")
	gitOk := isCommandAvailable("git", "--version")
	if !gitOk {
		fmt.Println("❌ Git이 설치되어 있지 않거나 PATH에 없습니다.")
		fmt.Print("Git 자동 설치를 시도하시겠습니까? (y/n): ")
		if strings.ToLower(strings.TrimSpace(getUserChoice())) == "y" {
			if installGit() {
				fmt.Println("✅ Git 설치 완료. PATH 적용을 위해 이 프로그램을 재시작해야 할 수 있습니다.")
			} else {
				fmt.Println("❌ Git 자동 설치에 실패했습니다. 수동으로 설치 후 다시 실행해주세요.")
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
				fmt.Println("✅ Node.js 설치 완료. PATH 적용을 위해 이 프로그램을 재시작해야 할 수 있습니다.")
			} else {
				fmt.Println("❌ Node.js 자동 설치에 실패했습니다. 수동으로 설치 후 다시 실행해주세요.")
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
}

func isCommandAvailable(cmd string, args ...string) bool {
	command := exec.Command(cmd, args...)
	return command.Run() == nil
}

// Git Stash 자동 복원 시도 함수
func tryApplyStash(repoPath string) (bool, string) {
	// Stash 목록 확인
	checkStashCmd := exec.Command("git", "-C", repoPath, "stash", "list")
	stashListOutput, err := checkStashCmd.Output()
	if err != nil { // 오류 발생 시 (e.g. git repo 아님)
		return false, fmt.Sprintf("stash 목록 확인 실패: %v", err)
	}
	if strings.TrimSpace(string(stashListOutput)) == "" {
		fmt.Println("ℹ️ 적용할 로컬 변경사항(stash)이 없습니다.")
		return true, "적용할 stash 없음" // Stash가 없으면 성공으로 간주
	}

	fmt.Println("저장된 로컬 변경사항(stash) 자동 복원 시도 (git stash pop)...")
	// git stash pop 실행, 표준 출력과 표준 에러를 함께 캡처
	cmd := exec.Command("git", "-C", repoPath, "stash", "pop")
	var out<y_bin_441> bytes.Buffer
	cmd.Stdout = &out<y_bin_441>
	cmd.Stderr = &out<y_bin_441>

	err = cmd.Run()
	output := out<y_bin_441>.String()

	if err != nil {
		// "git stash pop" 실패 (대부분 충돌 때문)
		fmt.Println("\n❌ 자동 복원 실패: 로컬 변경사항을 다시 적용하는 중 문제가 발생했습니다.")
		fmt.Println("Git 메시지:")
		fmt.Println("---------------------------------------------------------")
		fmt.Println(output)
		fmt.Println("---------------------------------------------------------")
		fmt.Println("ℹ️ 충돌(Conflict)이 발생했을 수 있습니다. 이 경우 수동으로 해결해야 합니다.")
		fmt.Println("   터미널에서 다음 단계를 따르세요:")
		fmt.Println("   1. SillyTavern 폴더로 이동합니다 (예: cd " + repoPath + ")")
		fmt.Println("   2. 'git status' 명령으로 충돌이 발생한 파일 목록을 확인하세요.")
		fmt.Println("   3. 각 충돌 파일을 열어 '<<<<<', '=====', '>>>>>' 마커를 참고하여 직접 수정하세요.")
		fmt.Println("   4. 수정 후 'git add <파일명>' 명령으로 해결된 파일을 스테이징하세요.")
		fmt.Println("   5. 모든 충돌 해결 후 'git commit' 명령으로 커밋하세요 (커밋 메시지는 'Merge stash' 등).")
		fmt.Println("   6. 그 후, 'git stash drop' 명령으로 적용 시도했던 stash 항목을 삭제하세요 (이미 pop 시도 중 실패하면 자동으로 drop되지 않음).")
		fmt.Println("   만약 복원을 완전히 취소하고 stash 이전 상태로 돌아가고 싶다면:")
		fmt.Println("   'git reset --hard HEAD' 실행 후, 'git stash drop'으로 해당 stash를 제거하세요.")
		fmt.Println("   (참고: 'git stash list'로 남아있는 stash를 확인할 수 있습니다.)")
		return false, output
	}

	fmt.Println("\n✅ 저장된 로컬 변경사항(stash)이 성공적으로 복원/적용되었습니다.")
	if strings.Contains(output, "No stash entries found") { // stash pop은 성공했지만 실제론 아무것도 없었을때 (거의 발생 안함)
		fmt.Println("   (실제 적용된 변경사항은 없었습니다.)")
	} else if strings.Contains(output, "Dropped refs") { // 정상적으로 pop되고 stash가 drop 된 경우
		// 특별한 메시지 없이 성공으로 간주
	}
	// fmt.Println("Git 메시지:", output) // 상세 메시지 필요시 주석 해제
	return true, output
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
			// 기본 브랜치로 진행하거나 사용자에게 선택을 요구할 수 있음. 여기서는 업데이트 시도.
		}
	}

	if !stDirExists {
		fmt.Printf("%s 디렉토리에 실리태번을 새로 설치합니다 (기본 브랜치: %s)...\n", baseDir, defaultBranch)
		cloneRepo(baseDir, defaultBranch)
		// 최초 클론 후에는 stash가 없으므로 tryApplyStash 호출 불필요
	} else {
		if currentBranch == "" {
			fmt.Printf("⚠️ 현재 브랜치를 알 수 없어 기본 브랜치(%s) 기준으로 업데이트합니다.\n", defaultBranch)
			currentBranch = defaultBranch // fallback
		}
		fmt.Printf("%s 디렉토리의 실리태번을 업데이트합니다 (브랜치: %s)...\n", baseDir, currentBranch)
		updateRepo(baseDir, currentBranch) // updateRepo 내부에서 tryApplyStash 호출
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
	os.Chdir(baseDir)
	defer os.Chdir(originalWd)

	fmt.Println("로컬 변경사항 임시 저장 (git stash push -u)...")
	stashCmd := exec.Command("git", "stash", "push", "-u", "-m", "AutoStash_BeforeUpdate_"+time.Now().Format("20060102150405"))
	stashOutput, _ := stashCmd.CombinedOutput() // 오류 발생해도 계속 진행, stash 결과는 참고용
	if strings.Contains(string(stashOutput), "No local changes to save") {
		fmt.Println("ℹ️ 임시 저장할 로컬 변경사항이 없습니다.")
	} else {
		fmt.Println("로컬 변경사항 임시 저장 완료.")
	}


	fmt.Println("원격 저장소 정보 가져오기 (git fetch origin)...")
	fetchCmd := exec.Command("git", "fetch", "origin")
	fetchCmd.Stdout = os.Stdout
	fetchCmd.Stderr = os.Stderr
	if err := fetchCmd.Run(); err != nil {
		fmt.Println("\n❌ 원격 저장소 정보 가져오기에 실패했습니다:", err)
		// 실패해도 pull 시도
	}

	if branchToUpdate == "" {
		fmt.Println("\n❌ 업데이트할 브랜치 정보가 없습니다.")
		return // 또는 기본 브랜치로 설정
	}

	fmt.Printf("브랜치 (%s) 를 원격 저장소(origin/%s) 기준으로 업데이트 (git pull origin %s)...\n", branchToUpdate, branchToUpdate, branchToUpdate)
	pullCmd := exec.Command("git", "pull", "origin", branchToUpdate)
	pullCmd.Stdout = os.Stdout
	pullCmd.Stderr = os.Stderr
	if err := pullCmd.Run(); err != nil {
		fmt.Println("\n❌ 저장소 업데이트(pull)에 실패했습니다:", err)
		fmt.Println("ℹ️  만약 로컬 변경사항과 충돌이 발생했다면, 수동으로 해결해야 할 수 있습니다.")
		fmt.Println("   'git status'로 상태를 확인하고, 충돌 해결 후 'git add .' -> 'git commit' 또는 'git pull'을 재시도하세요.")
		// 여기서 자동 stash pop 시도하지 않음 (pull 자체가 실패했으므로)
	} else {
		fmt.Println("✅ 저장소 업데이트 완료.")
		// Pull 성공 후 Stash 적용 시도
		tryApplyStash(".") // 현재 디렉토리 (baseDir)
	}
}

func installSillyTavernDependencies(baseDir string) {
	fmt.Println("\nSillyTavern에 필요한 패키지 설치 중 (npm install)... (시간이 다소 걸릴 수 있습니다)")
	originalWd, _ := os.Getwd()
	os.Chdir(baseDir)
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
	cmd := exec.Command("git", "-C", repoPath, "branch", "--show-current")
	branchBytes, err := cmd.Output()
	if err != nil {
		cmdRef := exec.Command("git", "-C", repoPath, "symbolic-ref", "--short", "HEAD")
		branchBytes, err = cmdRef.Output()
		if err != nil {
			return "", fmt.Errorf("현재 브랜치 확인 실패: %w", err)
		}
	}
	currentBranch := strings.TrimSpace(string(branchBytes))
	if currentBranch == "" {
		return "", fmt.Errorf("현재 브랜치를 확인할 수 없음 (결과 비어있음)")
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
	case "1": targetBranch = defaultBranch
	case "2": targetBranch = stagingBranch
	default:
		fmt.Println("\n잘못된 선택입니다.")
		return
	}

	originalWd, _ := os.Getwd()
	os.Chdir(baseDir) // SillyTavern 디렉토리 내에서 git 명령어 실행
	defer os.Chdir(originalWd)

	currentBranch, err := getCurrentGitBranch(".")
	if err != nil {
		fmt.Printf("\n⚠️ 현재 브랜치를 확인하는데 실패했습니다 (오류 무시하고 진행): %v\n", err)
	}

	if currentBranch == targetBranch {
		fmt.Printf("\n이미 %s 브랜치를 사용 중입니다. 최신 버전으로 업데이트를 시도합니다...\n", targetBranch)
		updateRepo(".", targetBranch) // 현재 디렉토리(baseDir) 및 대상 브랜치
		installSillyTavernDependencies(".")
		return
	}

	fmt.Printf("\n%s 브랜치로 전환 중...\n", targetBranch)
	fmt.Println("브랜치 전환 전 로컬 변경사항 임시 저장 (git stash push -u)...")
	stashCmd := exec.Command("git", "stash", "push", "-u", "-m", "AutoStash_BeforeBranchSwitch_"+time.Now().Format("20060102150405"))
	stashOutput, _ := stashCmd.CombinedOutput()
	stashedSomethingBeforeSwitch := !strings.Contains(string(stashOutput), "No local changes to save")
	if !stashedSomethingBeforeSwitch {
		fmt.Println("ℹ️ 임시 저장할 로컬 변경사항이 없었습니다 (브랜치 전환 전).")
	} else {
		fmt.Println("로컬 변경사항 임시 저장 완료 (브랜치 전환 전).")
	}


	fmt.Printf("원격 저장소에서 %s 브랜치 정보 가져오기 (git fetch origin %s)...\n", targetBranch, targetBranch)
	fetchBranchCmd := exec.Command("git", "fetch", "origin", targetBranch+":"+targetBranch)
	if err := fetchBranchCmd.Run(); err != nil {
		fmt.Printf("\n⚠️ 원격 브랜치(%s)를 가져오는데 실패했을 수 있습니다: %v\n", targetBranch, err)
	}

	fmt.Printf("브랜치 전환 (git checkout %s)...\n", targetBranch)
	checkoutCmd := exec.Command("git", "checkout", targetBranch)
	checkoutCmd.Stdout = os.Stdout
	checkoutCmd.Stderr = os.Stderr
	if err := checkoutCmd.Run(); err != nil {
		fmt.Println("\n❌ 브랜치 전환에 실패했습니다:", err)
		fmt.Println("ℹ️  'git stash list'를 확인하고 'git stash pop'으로 임시 저장된 변경사항을 복원해 보세요.")
		// 전환 실패시 stashedSomethingBeforeSwitch 가 true 이면 pop 시도해볼 여지 남김.
		if stashedSomethingBeforeSwitch {
			fmt.Println("   전환 전 임시 저장했던 내용을 복원 시도할 수 있습니다.")
		}
	} else {
		fmt.Printf("\n✅ %s 브랜치로 전환 완료!\n", targetBranch)
		fmt.Println("전환된 브랜치 최신화 (git pull origin)...")
		updateRepo(".", targetBranch) // updateRepo 내부에서 pull 후 stash pop 시도
		
		// 브랜치 전환 "전"의 stash를 적용할지 여부
		// updateRepo는 해당 브랜치의 최신화 "전"의 변경사항을 stash하고 pop함.
		// 지금 적용할 stash는 원래 브랜치에서 가져온 변경사항임.
		if stashedSomethingBeforeSwitch {
			fmt.Println("\n이전 브랜치에서 가져온 로컬 변경사항 자동 복원 시도...")
			tryApplyStash(".") 
		}
		installSillyTavernDependencies(".")
	}
}


func runSillyTavern() {
	baseDir := defaultBaseDir
	sillyTavernPath := filepath.Join(".", baseDir)
	absPath, _ := filepath.Abs(sillyTavernPath) // 로그용 절대경로

	if _, err := os.Stat(sillyTavernPath); os.IsNotExist(err) {
		fmt.Printf("\n❌ 실리태번이 설치되어 있지 않습니다. (검색 경로: %s)\n", absPath)
		return
	}

	fmt.Println("\n실리태번을 실행합니다...")
	startScript := "start.bat"
	scriptPath := filepath.Join(sillyTavernPath, startScript)

	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		fmt.Printf("\n❌ 실행 파일(%s)을 찾을 수 없습니다. (경로: %s)\n", startScript, scriptPath)
		return
	}

	cmd := exec.Command("cmd", "/c", "start", "\"SillyTavern Launcher\"", "/D", "\""+sillyTavernPath+"\"", "\""+startScript+"\"")
	if err := cmd.Start(); err != nil {
		fmt.Println("\n❌ 실리태번 실행에 실패했습니다:", err)
	} else {
		fmt.Println("\n✅ 실리태번이 새 창에서 실행 요청되었습니다!")
		fmt.Println("   SillyTavern 실행 후 이 창은 닫으셔도 됩니다.")
	}
}

func installProgram(name, wingetID, chocoPkg, downloadURL, installerName, installerArgs string, versionCheckCmds [][]string) bool {
	fmt.Printf("%s 자동 설치 시도...\n", name)

	// 1. Winget
	if isCommandAvailable(wingetCheckCommand, "--version") {
		fmt.Printf("Winget으로 %s 설치 중... (winget install -e --id %s)\n", name, wingetID)
		wingetCmd := exec.Command(wingetCheckCommand, "install", "--id", wingetID, "-e", "--accept-source-agreements", "--accept-package-agreements")
		wingetCmd.Stdout = os.Stdout
		wingetCmd.Stderr = os.Stderr
		if err := wingetCmd.Run(); err == nil {
			fmt.Printf("%s 설치 시도 완료 (Winget). 적용 확인 중...\n", name)
			time.Sleep(3 * time.Second)
			refreshEnv()
			allCmdsOk := true
			for _, vc := range versionCheckCmds {
				if !isCommandAvailable(vc[0], vc[1]) { allCmdsOk = false; break }
			}
			if allCmdsOk { return true }
			fmt.Printf("Winget 설치 후에도 %s 명령어를 찾을 수 없습니다.\n", name)
		} else {
			fmt.Printf("Winget을 사용한 %s 설치 실패: %v\n", name, err)
		}
	}

	// 2. Chocolatey
	if isCommandAvailable(chocolateyCheckCommand, "--version") {
		fmt.Printf("Chocolatey로 %s 설치 중... (choco install %s -y)\n", name, chocoPkg)
		chocoCmd := exec.Command(chocolateyCheckCommand, "install", chocoPkg, "-y")
		chocoCmd.Stdout = os.Stdout
		chocoCmd.Stderr = os.Stderr
		if err := chocoCmd.Run(); err == nil {
			fmt.Printf("%s 설치 시도 완료 (Chocolatey). 적용 확인 중...\n", name)
			time.Sleep(3 * time.Second)
			refreshEnv()
			allCmdsOk := true
			for _, vc := range versionCheckCmds {
				if !isCommandAvailable(vc[0], vc[1]) { allCmdsOk = false; break }
			}
			if allCmdsOk { return true }
			fmt.Printf("Chocolatey 설치 후에도 %s 명령어를 찾을 수 없습니다.\n", name)
		} else {
			fmt.Printf("Chocolatey를 사용한 %s 설치 실패: %v\n", name, err)
		}
	}
	
	// 3. Direct Download
	if downloadURL != "" && installerName != "" {
		fmt.Printf("%s 설치 파일 다운로드 시도: %s\n", name, downloadURL)
		installerPath := filepath.Join(os.TempDir(), installerName)
		
		if err := downloadFile(downloadURL, installerPath); err != nil {
			fmt.Printf("%s 다운로드 실패: %v\n", name, err)
			return false
		}

		fmt.Printf("%s 설치 중... (자동 실행, UAC 팝업 가능)\n", name)
		var installCmd *exec.Cmd
		if strings.HasSuffix(installerName, ".msi") {
			installCmd = exec.Command("msiexec", "/i", installerPath, "/quiet", "/norestart")
		} else { // .exe
			args := strings.Fields(installerArgs) // 공백 기준으로 인수 분리
			fullArgs := append([]string{installerPath}, args...)
			installCmd = exec.Command(fullArgs[0], fullArgs[1:]...)
		}
		
		if err := installCmd.Run(); err != nil {
			fmt.Printf("%s 설치 프로그램 실행 실패: %v\n", name, err)
			os.Remove(installerPath) // 실패 시에도 다운로드 파일 정리
			return false
		}
		os.Remove(installerPath) // 성공 시 다운로드 파일 정리

		fmt.Printf("%s 설치 완료 (직접 설치). 적용 중...\n", name)
		time.Sleep(5 * time.Second)
		refreshEnv()
		allCmdsOk := true
		for _, vc := range versionCheckCmds {
			if !isCommandAvailable(vc[0], vc[1]) { allCmdsOk = false; break }
		}
		if allCmdsOk { return true }
		fmt.Printf("직접 설치 후에도 %s 명령어를 찾을 수 없습니다. 시스템 재시작 필요 가능성.\n", name)
	}
	return false
}

func installGit() bool {
	return installProgram("Git", "Git.Git", "git.install", gitForWindowsURL, "git_installer.exe", "/VERYSILENT /NORESTART /NOCANCEL /SP- /CLOSEAPPLICATIONS /RESTARTAPPLICATIONS /MERGETASKS=!desktopicon", [][]string{{"git", "--version"}})
}

func installNodeJS() bool {
	return installProgram("Node.js LTS", "OpenJS.NodeJS.LTS", "nodejs-lts", nodeJSWindowsURL, "nodejs_lts_installer.msi", "/quiet /norestart", [][]string{{"node", "--version"}, {"npm", "--version"}})
}


func downloadFile(url, targetFilepath string) error {
	fmt.Printf("다운로드 시작: %s -> %s\n", url, targetFilepath)
	dir := filepath.Dir(targetFilepath)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("임시 디렉토리(%s) 생성 실패: %w", dir, err)
		}
	}

	client := http.Client{ CheckRedirect: func(req *http.Request, via []*http.Request) error { if len(via) >= 10 { return fmt.Errorf("stopped after 10 redirects"); }; return nil; } }
	resp, err := client.Get(url)
	if err != nil { return fmt.Errorf("HTTP GET 실패 (%s): %w", url, err) }
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("잘못된 응답 상태코드 (%s): %s. 응답: %s", url, resp.Status, string(bodyBytes))
	}

	out, err := os.Create(targetFilepath)
	if err != nil { return fmt.Errorf("파일 생성 실패 (%s): %w", targetFilepath, err) }
	defer out.Close()

	fmt.Println("다운로드 중...")
	size, err := io.Copy(out, resp.Body)
	if err != nil { os.Remove(targetFilepath); return fmt.Errorf("파일 내용 복사 실패 (%s): %w", targetFilepath, err); }
	fmt.Printf("다운로드 완료: %s (%.2f MB)\n", filepath.Base(targetFilepath), float64(size)/(1024*1024))
	return nil
}

func refreshEnv() {
	fmt.Println("환경 변수 새로고침 시도 (시스템 PATH 업데이트 반영 목적)...")
	cmdChocoRefresh := exec.Command("refreshenv.bat") // Chocolatey 사용자용
	errChoco := cmdChocoRefresh.Run()

	if errChoco != nil {
		fmt.Println("⚠️ 'refreshenv.bat' 실행 실패 또는 찾을 수 없음. (오류:", errChoco, ")")
		fmt.Println("   PATH 변경사항이 현재 세션에 즉시 적용되지 않을 수 있습니다.")
		fmt.Println("   이 프로그램을 재시작하거나, 새 명령 프롬프트 창을 열거나, 시스템 재부팅이 필요할 수 있습니다.")
	} else {
		fmt.Println("✅ 'refreshenv.bat' 실행 완료. 환경 변수가 새로고침되었을 수 있습니다.")
	}
	time.Sleep(1 * time.Second) 
}

func waitForExit() {
	fmt.Println("\n오류가 발생하여 프로그램을 계속 진행할 수 없습니다.")
	fmt.Println("자세한 오류 메시지는 위 내용을 참고하세요.")
	fmt.Println("종료하려면 엔터를 누르세요...")
	bufio.NewReader(os.Stdin).ReadString('\n')
	os.Exit(1)
}