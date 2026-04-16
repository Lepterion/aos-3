package main

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// --- Типи та константи ---

type OpCode uint16
type Register uint16

const (
	R0 Register = 0
	R1 Register = 1
	R2 Register = 2
	R3 Register = 3
	R4 Register = 4
	R5 Register = 5
	R6 Register = 6
	R7 Register = 7
)

const (
	OpBr   OpCode = 0  // 0000
	OpAdd  OpCode = 1  // 0001
	OpLd   OpCode = 2  // 0010
	OpSt   OpCode = 3  // 0011
	OpJsr  OpCode = 4  // 0100
	OpAnd  OpCode = 5  // 0101
	OpLdr  OpCode = 6  // 0110
	OpStr  OpCode = 7  // 0111
	OpRti  OpCode = 8  // 1000
	OpNot  OpCode = 9  // 1001
	OpLdi  OpCode = 10 // 1010
	OpSti  OpCode = 11 // 1011
	OpJmp  OpCode = 12 // 1100
	OpLea  OpCode = 14 // 1110
	OpTrap OpCode = 15 // 1111
)

const (
	TrapGetc  = 0x20 // Зчитати один символ (без виводу на екран)
	TrapOut   = 0x21 // Вивести символ з R0 на екран
	TrapPuts  = 0x22 // Вивести рядок (указівник у R0)
	TrapIn    = 0x23 // Зчитати символ з виводом на екран (echo)
	TrapPutsp = 0x24 // Вивести запакований рядок
	TrapHalt  = 0x25 // Зупинити віртуальну машину

	TrapInU16  = 0x26 // Зчитати uint16 з клавіатури у R0
	TrapOutU16 = 0x27 // Вивести uint16 з R0 у консоль
)

type Instruction struct {
	Code uint16
	Text string
}

// --- Двопрохідний компілятор ---

// Statement представляє або Мітку (Label), або Інструкцію
type Statement struct {
	IsLabel bool
	Name    string
	Resolve func(pc int, labels map[string]int) (Instruction, error)
}

// Label створює мітку в коді. Мітки не займають місця у скомпільованому бінарному файлі.
func Label(name string) Statement {
	return Statement{IsLabel: true, Name: name}
}

// Compile виконує два проходи й перетворює список Statement на готовий до запису масив Instruction.
func Compile(stmts []Statement) ([]Instruction, error) {
	labels := make(map[string]int)
	var program []Instruction
	pc := 0

	// ПЕРШИЙ ПРОХІД: Збираємо адреси міток
	for _, stmt := range stmts {
		if stmt.IsLabel {
			labels[stmt.Name] = pc
		} else {
			pc++
		}
	}

	// ДРУГИЙ ПРОХІД: Генеруємо машинний код
	pc = 0
	for _, stmt := range stmts {
		if stmt.IsLabel {
			continue
		}
		instr, err := stmt.Resolve(pc, labels)
		if err != nil {
			return nil, fmt.Errorf("помилка на PC=%d: %v", pc, err)
		}
		program = append(program, instr)
		pc++
	}

	return program, nil
}

// --- Інструкції з підтримкою міток (PC-relative) ---

// LD (Load) завантажує значення з пам'яті у вказаний регістр, використовуючи PC-відносне зміщення до мітки (dst = mem[label]).
func LD(dst Register, labelName string) Statement {
	return Statement{Resolve: func(pc int, labels map[string]int) (Instruction, error) {
		target, ok := labels[labelName]
		if !ok {
			return Instruction{}, fmt.Errorf("мітку '%s' не знайдено", labelName)
		}
		offset9 := target - (pc + 1)
		code := (uint16(OpLd) << 12) | (uint16(dst) << 9) | (uint16(offset9) & 0x01FF)
		return Instruction{code, fmt.Sprintf("LD R%d, %s", dst, labelName)}, nil
	}}
}

// ST (Store) зберігає значення з регістра у пам'ять, використовуючи PC-відносне зміщення до мітки (mem[label] = src).
func ST(src Register, labelName string) Statement {
	return Statement{Resolve: func(pc int, labels map[string]int) (Instruction, error) {
		target, ok := labels[labelName]
		if !ok {
			return Instruction{}, fmt.Errorf("мітку '%s' не знайдено", labelName)
		}
		offset9 := target - (pc + 1)
		code := (uint16(OpSt) << 12) | (uint16(src) << 9) | (uint16(offset9) & 0x01FF)
		return Instruction{code, fmt.Sprintf("ST R%d, %s", src, labelName)}, nil
	}}
}

// BR (Branch) виконує умовний перехід на вказану мітку, якщо встановлені відповідні прапорці (n - negative, z - zero, p - positive).
func BR(n, z, p bool, labelName string) Statement {
	return Statement{Resolve: func(pc int, labels map[string]int) (Instruction, error) {
		target, ok := labels[labelName]
		if !ok {
			return Instruction{}, fmt.Errorf("мітку '%s' не знайдено", labelName)
		}
		offset9 := target - (pc + 1)
		var nBit, zBit, pBit uint16
		cond := ""
		if n {
			nBit = 1
			cond += "n"
		}
		if z {
			zBit = 1
			cond += "z"
		}
		if p {
			pBit = 1
			cond += "p"
		}
		code := (uint16(OpBr) << 12) | (nBit << 11) | (zBit << 10) | (pBit << 9) | (uint16(offset9) & 0x01FF)
		return Instruction{code, fmt.Sprintf("BR%s %s", cond, labelName)}, nil
	}}
}

// LEA (Load Effective Address) обчислює адресу мітки та завантажує її у вказаний регістр (dst = адреса мітки).
func LEA(dst Register, labelName string) Statement {
	return Statement{Resolve: func(pc int, labels map[string]int) (Instruction, error) {
		target, ok := labels[labelName]
		if !ok {
			return Instruction{}, fmt.Errorf("мітку '%s' не знайдено", labelName)
		}
		offset9 := target - (pc + 1)
		code := (uint16(OpLea) << 12) | (uint16(dst) << 9) | (uint16(offset9) & 0x01FF)
		return Instruction{code, fmt.Sprintf("LEA R%d, %s", dst, labelName)}, nil
	}}
}

// LDI (Load Indirect) завантажує значення з пам'яті непрямо. Спочатку береться адреса за міткою, а потім значення за цією адресою (dst = mem[mem[label]]).
func LDI(dst Register, labelName string) Statement {
	return Statement{Resolve: func(pc int, labels map[string]int) (Instruction, error) {
		target, ok := labels[labelName]
		if !ok {
			return Instruction{}, fmt.Errorf("мітку '%s' не знайдено", labelName)
		}
		offset9 := target - (pc + 1)
		code := (uint16(OpLdi) << 12) | (uint16(dst) << 9) | (uint16(offset9) & 0x01FF)
		return Instruction{code, fmt.Sprintf("LDI R%d, %s", dst, labelName)}, nil
	}}
}

// STI (Store Indirect) зберігає значення у пам'ять непрямо. Спочатку береться адреса за міткою, а потім туди записується значення з регістра (mem[mem[label]] = src).
func STI(src Register, labelName string) Statement {
	return Statement{Resolve: func(pc int, labels map[string]int) (Instruction, error) {
		target, ok := labels[labelName]
		if !ok {
			return Instruction{}, fmt.Errorf("мітку '%s' не знайдено", labelName)
		}
		offset9 := target - (pc + 1)
		code := (uint16(OpSti) << 12) | (uint16(src) << 9) | (uint16(offset9) & 0x01FF)
		return Instruction{code, fmt.Sprintf("STI R%d, %s", src, labelName)}, nil
	}}
}

// JSR (Jump to Subroutine) викликає підпрограму за міткою. Зберігає адресу повернення (PC) у регістр R7.
func JSR(labelName string) Statement {
	return Statement{Resolve: func(pc int, labels map[string]int) (Instruction, error) {
		target, ok := labels[labelName]
		if !ok {
			return Instruction{}, fmt.Errorf("мітку '%s' не знайдено", labelName)
		}
		offset11 := target - (pc + 1)
		code := (uint16(OpJsr) << 12) | (1 << 11) | (uint16(offset11) & 0x07FF)
		return Instruction{code, fmt.Sprintf("JSR %s", labelName)}, nil
	}}
}

// --- Інструкції без міток ---

// AddReg (Add Register) додає значення двох регістрів та зберігає результат у цільовий регістр (dst = src1 + src2).
func AddReg(dst, src1, src2 Register) Statement {
	return Statement{Resolve: func(pc int, labels map[string]int) (Instruction, error) {
		code := (uint16(OpAdd) << 12) | (uint16(dst) << 9) | (uint16(src1) << 6) | uint16(src2)
		return Instruction{code, fmt.Sprintf("ADD R%d, R%d, R%d", dst, src1, src2)}, nil
	}}
}

// AddImm (Add Immediate) додає 5-бітну константу (від -16 до +15) до значення регістра та зберігає результат (dst = src1 + imm5).
func AddImm(dst, src1 Register, imm5 int16) Statement {
	return Statement{Resolve: func(pc int, labels map[string]int) (Instruction, error) {
		code := (uint16(OpAdd) << 12) | (uint16(dst) << 9) | (uint16(src1) << 6) | 0x0020 | (uint16(imm5) & 0x001F)
		return Instruction{code, fmt.Sprintf("ADD R%d, R%d, #%d", dst, src1, imm5)}, nil
	}}
}

// AndReg (Bitwise AND Register) виконує побітове "І" між двома регістрами (dst = src1 & src2).
func AndReg(dst, src1, src2 Register) Statement {
	return Statement{Resolve: func(pc int, labels map[string]int) (Instruction, error) {
		code := (uint16(OpAnd) << 12) | (uint16(dst) << 9) | (uint16(src1) << 6) | uint16(src2)
		return Instruction{code, fmt.Sprintf("AND R%d, R%d, R%d", dst, src1, src2)}, nil
	}}
}

// AndImm (Bitwise AND Immediate) виконує побітове "І" між регістром та 5-бітною константою (dst = src1 & imm5).
func AndImm(dst, src1 Register, imm5 int16) Statement {
	return Statement{Resolve: func(pc int, labels map[string]int) (Instruction, error) {
		code := (uint16(OpAnd) << 12) | (uint16(dst) << 9) | (uint16(src1) << 6) | 0x0020 | (uint16(imm5) & 0x001F)
		return Instruction{code, fmt.Sprintf("AND R%d, R%d, #%d", dst, src1, imm5)}, nil
	}}
}

// NOT (Bitwise Complement) інвертує всі біти у вказаному регістрі (dst = ~src).
func NOT(dst, src Register) Statement {
	return Statement{Resolve: func(pc int, labels map[string]int) (Instruction, error) {
		code := (uint16(OpNot) << 12) | (uint16(dst) << 9) | (uint16(src) << 6) | 0x003F
		return Instruction{code, fmt.Sprintf("NOT R%d, R%d", dst, src)}, nil
	}}
}

// LDR (Load Base+Offset) завантажує значення з пам'яті, використовуючи базовий регістр та 6-бітне зміщення (dst = mem[baseR + offset6]).
func LDR(dst, baseR Register, offset6 int16) Statement {
	return Statement{Resolve: func(pc int, labels map[string]int) (Instruction, error) {
		code := (uint16(OpLdr) << 12) | (uint16(dst) << 9) | (uint16(baseR) << 6) | (uint16(offset6) & 0x003F)
		return Instruction{code, fmt.Sprintf("LDR R%d, R%d, #%d", dst, baseR, offset6)}, nil
	}}
}

// STR (Store Base+Offset) зберігає значення у пам'ять, використовуючи базовий регістр та 6-бітне зміщення (mem[baseR + offset6] = src).
func STR(src, baseR Register, offset6 int16) Statement {
	return Statement{Resolve: func(pc int, labels map[string]int) (Instruction, error) {
		code := (uint16(OpStr) << 12) | (uint16(src) << 9) | (uint16(baseR) << 6) | (uint16(offset6) & 0x003F)
		return Instruction{code, fmt.Sprintf("STR R%d, R%d, #%d", src, baseR, offset6)}, nil
	}}
}

// JMP (Jump) виконує безумовний перехід за адресою, що зберігається у вказаному базовому регістрі (PC = baseR).
func JMP(baseR Register) Statement {
	return Statement{Resolve: func(pc int, labels map[string]int) (Instruction, error) {
		code := (uint16(OpJmp) << 12) | (uint16(baseR) << 6)
		return Instruction{code, fmt.Sprintf("JMP R%d", baseR)}, nil
	}}
}

// RET (Return) повертає керування з підпрограми. Є псевдокомандою для JMP R7 (PC = R7).
func RET() Statement {
	return Statement{Resolve: func(pc int, labels map[string]int) (Instruction, error) {
		code := (uint16(OpJmp) << 12) | (uint16(R7) << 6)
		return Instruction{code, "RET"}, nil
	}}
}

// JSRR (Jump to Subroutine Register) викликає підпрограму за адресою у регістрі. Зберігає адресу повернення у R7 (R7 = PC, PC = baseR).
func JSRR(baseR Register) Statement {
	return Statement{Resolve: func(pc int, labels map[string]int) (Instruction, error) {
		code := (uint16(OpJsr) << 12) | (0 << 11) | (uint16(baseR) << 6)
		return Instruction{code, fmt.Sprintf("JSRR R%d", baseR)}, nil
	}}
}

// RTI (Return from Interrupt) повертає керування після обробки переривання (відновлює PC та PSR зі стека).
func RTI() Statement {
	return Statement{Resolve: func(pc int, labels map[string]int) (Instruction, error) {
		code := uint16(OpRti) << 12
		return Instruction{code, "RTI"}, nil
	}}
}

// TRAP викликає системну підпрограму (вектор переривання ОС), наприклад x25 для HALT (зупинка машини).
func TRAP(vector8 uint16) Statement {
	return Statement{Resolve: func(pc int, labels map[string]int) (Instruction, error) {
		code := (uint16(OpTrap) << 12) | (vector8 & 0x00FF)
		return Instruction{code, fmt.Sprintf("TRAP x%02X", vector8)}, nil
	}}
}

// Data розміщує сирі 16-бітні дані (наприклад, змінні) безпосередньо в пам'яті.
func Data(val uint16, comment string) Statement {
	return Statement{Resolve: func(pc int, labels map[string]int) (Instruction, error) {
		return Instruction{val, fmt.Sprintf(".FILL x%04X ; %s", val, comment)}, nil
	}}
}

// --- Головна програма ---

func main() {
	var valA, valB uint16

	fmt.Println("=== Генератор коду LC-3 для задачі: a * (1 - b) ===")

	// Запитуємо користувача про значення
	fmt.Print("Введіть значення для a (додатне ціле): ")
	if _, err := fmt.Scanf("%d", &valA); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Помилка вводу: %v\n", err)
		return
	}

	fmt.Print("Введіть значення для b (додатне ціле): ")
	// Очищуємо буфер після попереднього уводу, щоб уникнути проблем зі знаком нового рядка
	_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
	if _, err := fmt.Scanf("%d", &valB); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Помилка вводу: %v\n", err)
		return
	}

	programStmts := []Statement{
		LD(R0, "VAL_A"),
		LD(R1, "VAL_B"),

		AddImm(R1, R1, -1),

		AndImm(R2, R2, 0),
		AddImm(R1, R1, 0),
		BR(true, true, false, "DONE"),

		Label("LOOP"),
		AddReg(R2, R2, R0),
		AddImm(R1, R1, -1),
		BR(false, false, true, "LOOP"),

		Label("DONE"),
		NOT(R2, R2),
		AddImm(R2, R2, 1),

		ST(R2, "RES"),

		AddImm(R0, R2, 0),
		TRAP(TrapOutU16),

		TRAP(TrapHalt),

		// Змінні в пам'яті
		Label("VAL_A"), Data(valA, fmt.Sprintf("a = %d", valA)),
		Label("VAL_B"), Data(valB, fmt.Sprintf("b = %d", valB)),
		Label("RES"), Data(0x0000, "результат"),
	}

	// Компілюємо програму
	program, err := Compile(programStmts)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Помилка компіляції: %v\n", err)
		return
	}

	// --- Вивід у консоль ---
	fmt.Println("\nЗгенерований код для LC-3:")
	fmt.Println("----------------------------------------------------------------------")
	fmt.Printf("%-5s | %-6s | %-19s | %s\n", "Адр.", "Hex", "Binary", "Асемблер")
	fmt.Println("----------------------------------------------------------------------")

	var binProgram []uint16
	for i, instr := range program {
		binStr := fmt.Sprintf("%016b", instr.Code)
		formattedBin := fmt.Sprintf("%s %s %s %s", binStr[0:4], binStr[4:8], binStr[8:12], binStr[12:16])
		fmt.Printf("%02d    | 0x%04X | %s | %s\n", i, instr.Code, formattedBin, instr.Text)
		binProgram = append(binProgram, instr.Code)
	}
	fmt.Println("----------------------------------------------------------------------")

	// --- Запис у файл ---
	outputFile := "task.obj"
	f, err := os.Create(outputFile)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Помилка створення файлу: %v\n", err)
		return
	}

	err = binary.Write(f, binary.LittleEndian, binProgram)
	_ = f.Close()

	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Помилка запису: %v\n", err)
		return
	}

	fmt.Printf("Бінарний файл %s успішно згенеровано!\n", outputFile)

	if _, err := os.Stat("vm.exe"); err == nil {
		fmt.Print("\nЗнайдено vm.exe у поточній директорії. Бажаєте запустити програму? (y/N): ")

		var response string
		_, _ = fmt.Scan(&response)
		response = strings.ToLower(strings.TrimSpace(response))

		if response == "y" || response == "yes" || response == "н" || response == "так" {
			fmt.Println("\n=== Запуск vm.exe", outputFile, "===")

			cmd := exec.Command("./vm.exe", outputFile)

			// Перехоплюємо стандартний вивід ВМ
			stdoutPipe, err := cmd.StdoutPipe()
			if err != nil {
				_, _ = fmt.Fprintf(os.Stderr, "Помилка перехоплення виводу: %v\n", err)
				return
			}

			// Помилки та увід залишаємо прямими
			cmd.Stderr = os.Stderr
			cmd.Stdin = os.Stdin

			if err := cmd.Start(); err != nil {
				_, _ = fmt.Fprintf(os.Stderr, "\nПомилка запуску vm.exe: %v\n", err)
				return
			}

			foundResult := false
			var result int16

			// Читаємо вивід віртуальної машини рядок за рядком
			scanner := bufio.NewScanner(stdoutPipe)
			for scanner.Scan() {
				line := scanner.Text()

				// Якщо рядок не містить технічної інформації (mem або reg), ми перевіряємо, чи є в ньому число, виведене через TRAP
				if !strings.Contains(line, "mem[") && !strings.Contains(line, "reg[") && !strings.Contains(line, "memory") {
					// Спробуємо зчитати рядок як беззнакове число (uint16)
					cleanLine := strings.TrimSpace(line)
					if val, err := strconv.ParseUint(cleanLine, 10, 16); err == nil {
						signedVal := int16(uint16(val))
						fmt.Printf("%s\t<-- Результат (У знаковому виді: %d)\n", line, signedVal)
						result = signedVal
						foundResult = true
						continue
					}
				}

				// Якщо це звичайний рядок (або дамп пам'яті), просто друкуємо його
				fmt.Println(line)
			}

			_ = cmd.Wait()
			fmt.Println("\n=== Роботу віртуальної машини завершено ===")

			if foundResult {
				fmt.Printf("Зчитаний результат: %d\n", result)
			} else {
				fmt.Printf("Результат не знайдено у виводі віртуальної машини. Помилка?")
			}
		} else {
			fmt.Println("Запуск скасовано.")
		}
	}
}
