package igb

import (
	"bytes"
	"fmt"
	"github.com/goby-lang/goby/compiler/bytecode"
	"github.com/goby-lang/goby/compiler/lexer"
	"github.com/goby-lang/goby/compiler/parser"
	"github.com/goby-lang/goby/vm"
	"io"
	"os"

	"github.com/goby-lang/goby/Godeps/_workspace/src/github.com/looplab/fsm"
)

const (
	prompt = ">> "

	readyToExec = "readyToExec"
	Waiting     = "waiting"
	waitEnded   = "waitEnded"
)

var sm = fsm.NewFSM(
	readyToExec,
	fsm.Events{
		{Name: Waiting, Src: []string{waitEnded, readyToExec}, Dst: Waiting},
		{Name: waitEnded, Src: []string{Waiting}, Dst: waitEnded},
		{Name: readyToExec, Src: []string{waitEnded, readyToExec}, Dst: readyToExec},
	},
	fsm.Callbacks{},
)
var stmts = bytes.Buffer{}

// Start starts goby's REPL.
func Start(ch chan string, out io.Writer) {
	// Initialize VM
	v := vm.New(os.Getenv("GOBY_ROOT"), []string{})
	v.SetClassISIndexTable("")
	v.SetMethodISIndexTable("")
	v.InitForREPL()

	// Initialize parser, lexer is not important here
	l := lexer.New("")
	p := parser.New(l)
	program, _ := p.ParseProgram()

	// Initialize code generator, and it will behavior a little different in REPL mode.
	g := bytecode.NewGenerator()
	g.REPL = true
	g.InitTopLevelScope(program)

	for {
		out.Write([]byte(prompt))

		line := <-ch

		out.Write([]byte(line + "\n"))

		switch line {
		case "exit":
			return
		case "\n":
			continue
		}

		l := lexer.New(line)
		p.Lexer = l
		program, err := p.ParseProgram()

		if err != nil {
			if err.IsEOF() {
				if !sm.Is(Waiting) {
					sm.Event(Waiting)
				}

				appendStmt(line)
				continue
			}

			if err.IsUnexpectedEnd() {
				sm.Event(waitEnded)
				appendStmt(line)
			} else {
				fmt.Println(err.Message)
				continue
			}

		}

		if sm.Is(Waiting) {
			appendStmt(line)
			continue
		}

		if sm.Is(waitEnded) {
			l := lexer.New(stmts.String())
			p.Lexer = l

			// Test if current input can be properly parsed.
			program, err = p.ParseProgram()

			/*
			 This could mean there still are statements not ended, for example:

			 ```ruby
			 class Foo
			   def bar
			   end # This make state changes to WaitEnded
			 # But here still needs an "end"
			 ```
			*/

			if err != nil {
				if !err.IsEOF() {
					fmt.Println(err.Message)
				}
				continue
			}

			// If everything goes well, reset state and statements buffer
			sm.Event(readyToExec)
			stmts.Reset()
		}

		if sm.Is(readyToExec) {
			instructions := g.GenerateInstructions(program.Statements)
			g.ResetInstructionSets()
			v.REPLExec(instructions)

			r := v.GetREPLResult()

			switch r {
			case "\n", "":
				continue
			default:
				out.Write([]byte(fmt.Sprintf("#=> %s\n", r)))
			}
		}
	}
}

func appendStmt(line string) {
	stmts.WriteString(line + "\n")
}