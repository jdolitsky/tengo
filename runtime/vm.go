package runtime

import (
	"fmt"
	"sync/atomic"

	"github.com/d5/tengo/compiler"
	"github.com/d5/tengo/compiler/source"
	"github.com/d5/tengo/compiler/token"
	"github.com/d5/tengo/objects"
	"github.com/d5/tengo/stdlib"
)

const (
	// StackSize is the maximum stack size.
	StackSize = 2048

	// GlobalsSize is the maximum number of global variables.
	GlobalsSize = 1024

	// MaxFrames is the maximum number of function frames.
	MaxFrames = 1024
)

var (
	truePtr      = &objects.TrueValue
	falsePtr     = &objects.FalseValue
	undefinedPtr = &objects.UndefinedValue
	builtinFuncs []objects.Object
)

// VM is a virtual machine that executes the bytecode compiled by Compiler.
type VM struct {
	constants      []objects.Object
	stack          []*objects.Object
	sp             int
	globals        []*objects.Object
	fileSet        *source.FileSet
	frames         []Frame
	framesIndex    int
	curFrame       *Frame
	curInsts       []byte
	curIPLimit     int
	ip             int
	aborting       int64
	builtinModules map[string]*objects.Object
}

// NewVM creates a VM.
func NewVM(bytecode *compiler.Bytecode, globals []*objects.Object, builtinModules map[string]*objects.Object) *VM {
	if globals == nil {
		globals = make([]*objects.Object, GlobalsSize)
	}

	if builtinModules == nil {
		builtinModules = stdlib.Modules
	}

	frames := make([]Frame, MaxFrames)
	frames[0].fn = bytecode.MainFunction
	frames[0].freeVars = nil
	frames[0].ip = -1
	frames[0].basePointer = 0

	return &VM{
		constants:      bytecode.Constants,
		stack:          make([]*objects.Object, StackSize),
		sp:             0,
		globals:        globals,
		fileSet:        bytecode.FileSet,
		frames:         frames,
		framesIndex:    1,
		curFrame:       &(frames[0]),
		curInsts:       frames[0].fn.Instructions,
		curIPLimit:     len(frames[0].fn.Instructions) - 1,
		ip:             -1,
		builtinModules: builtinModules,
	}
}

// Abort aborts the execution.
func (v *VM) Abort() {
	atomic.StoreInt64(&v.aborting, 1)
}

// Run starts the execution.
func (v *VM) Run() error {
	// reset VM states
	v.sp = 0
	v.curFrame = &(v.frames[0])
	v.curInsts = v.curFrame.fn.Instructions
	v.curIPLimit = len(v.curInsts) - 1
	v.framesIndex = 1
	v.ip = -1
	atomic.StoreInt64(&v.aborting, 0)

mainloop:
	for v.ip < v.curIPLimit && (atomic.LoadInt64(&v.aborting) == 0) {
		v.ip++

		switch v.curInsts[v.ip] {
		case compiler.OpConstant:
			cidx := int(v.curInsts[v.ip+2]) | int(v.curInsts[v.ip+1])<<8
			v.ip += 2

			if v.sp >= StackSize {
				return ErrStackOverflow
			}

			v.stack[v.sp] = &v.constants[cidx]
			v.sp++

		case compiler.OpNull:
			if v.sp >= StackSize {
				return ErrStackOverflow
			}

			v.stack[v.sp] = undefinedPtr
			v.sp++

		case compiler.OpAdd:
			right := v.stack[v.sp-1]
			left := v.stack[v.sp-2]
			v.sp -= 2

			res, err := (*left).BinaryOp(token.Add, *right)
			if err != nil {
				filePos := v.fileSet.Position(v.curFrame.fn.SourceMap[v.ip])
				if err == objects.ErrInvalidOperator {
					return fmt.Errorf("%s: invalid operation: %s + %s",
						filePos, (*left).TypeName(), (*right).TypeName())
				}

				return fmt.Errorf("%s: %s", filePos, err.Error())
			}

			if v.sp >= StackSize {
				return ErrStackOverflow
			}

			v.stack[v.sp] = &res
			v.sp++

		case compiler.OpSub:
			right := v.stack[v.sp-1]
			left := v.stack[v.sp-2]
			v.sp -= 2

			res, err := (*left).BinaryOp(token.Sub, *right)
			if err != nil {
				filePos := v.fileSet.Position(v.curFrame.fn.SourceMap[v.ip])
				if err == objects.ErrInvalidOperator {
					return fmt.Errorf("%s: invalid operation: %s - %s",
						filePos, (*left).TypeName(), (*right).TypeName())
				}

				return fmt.Errorf("%s: %s", filePos, err.Error())
			}

			if v.sp >= StackSize {
				return ErrStackOverflow
			}

			v.stack[v.sp] = &res
			v.sp++

		case compiler.OpMul:
			right := v.stack[v.sp-1]
			left := v.stack[v.sp-2]
			v.sp -= 2

			res, err := (*left).BinaryOp(token.Mul, *right)
			if err != nil {
				filePos := v.fileSet.Position(v.curFrame.fn.SourceMap[v.ip])
				if err == objects.ErrInvalidOperator {
					return fmt.Errorf("%s: invalid operation: %s * %s",
						filePos, (*left).TypeName(), (*right).TypeName())
				}

				return fmt.Errorf("%s: %s", filePos, err.Error())
			}

			if v.sp >= StackSize {
				return ErrStackOverflow
			}

			v.stack[v.sp] = &res
			v.sp++

		case compiler.OpDiv:
			right := v.stack[v.sp-1]
			left := v.stack[v.sp-2]
			v.sp -= 2

			res, err := (*left).BinaryOp(token.Quo, *right)
			if err != nil {
				filePos := v.fileSet.Position(v.curFrame.fn.SourceMap[v.ip])
				if err == objects.ErrInvalidOperator {
					return fmt.Errorf("%s: invalid operation: %s / %s",
						filePos, (*left).TypeName(), (*right).TypeName())
				}

				return fmt.Errorf("%s: %s", filePos, err.Error())
			}

			if v.sp >= StackSize {
				return ErrStackOverflow
			}

			v.stack[v.sp] = &res
			v.sp++

		case compiler.OpRem:
			right := v.stack[v.sp-1]
			left := v.stack[v.sp-2]
			v.sp -= 2

			res, err := (*left).BinaryOp(token.Rem, *right)
			if err != nil {
				filePos := v.fileSet.Position(v.curFrame.fn.SourceMap[v.ip])
				if err == objects.ErrInvalidOperator {
					return fmt.Errorf("%s: invalid operation: %s %% %s",
						filePos, (*left).TypeName(), (*right).TypeName())
				}

				return fmt.Errorf("%s: %s", filePos, err.Error())
			}

			if v.sp >= StackSize {
				return ErrStackOverflow
			}

			v.stack[v.sp] = &res
			v.sp++

		case compiler.OpBAnd:
			right := v.stack[v.sp-1]
			left := v.stack[v.sp-2]
			v.sp -= 2

			res, err := (*left).BinaryOp(token.And, *right)
			if err != nil {
				filePos := v.fileSet.Position(v.curFrame.fn.SourceMap[v.ip])
				if err == objects.ErrInvalidOperator {
					return fmt.Errorf("%s: invalid operation: %s & %s",
						filePos, (*left).TypeName(), (*right).TypeName())
				}

				return fmt.Errorf("%s: %s", filePos, err.Error())
			}

			if v.sp >= StackSize {
				return ErrStackOverflow
			}

			v.stack[v.sp] = &res
			v.sp++

		case compiler.OpBOr:
			right := v.stack[v.sp-1]
			left := v.stack[v.sp-2]
			v.sp -= 2

			res, err := (*left).BinaryOp(token.Or, *right)
			if err != nil {
				filePos := v.fileSet.Position(v.curFrame.fn.SourceMap[v.ip])
				if err == objects.ErrInvalidOperator {
					return fmt.Errorf("%s: invalid operation: %s | %s",
						filePos, (*left).TypeName(), (*right).TypeName())
				}

				return fmt.Errorf("%s: %s", filePos, err.Error())
			}

			if v.sp >= StackSize {
				return ErrStackOverflow
			}

			v.stack[v.sp] = &res
			v.sp++

		case compiler.OpBXor:
			right := v.stack[v.sp-1]
			left := v.stack[v.sp-2]
			v.sp -= 2

			res, err := (*left).BinaryOp(token.Xor, *right)
			if err != nil {
				filePos := v.fileSet.Position(v.curFrame.fn.SourceMap[v.ip])
				if err == objects.ErrInvalidOperator {
					return fmt.Errorf("%s: invalid operation: %s ^ %s",
						filePos, (*left).TypeName(), (*right).TypeName())
				}

				return fmt.Errorf("%s: %s", filePos, err.Error())
			}

			if v.sp >= StackSize {
				return ErrStackOverflow
			}

			v.stack[v.sp] = &res
			v.sp++

		case compiler.OpBAndNot:
			right := v.stack[v.sp-1]
			left := v.stack[v.sp-2]
			v.sp -= 2

			res, err := (*left).BinaryOp(token.AndNot, *right)
			if err != nil {
				filePos := v.fileSet.Position(v.curFrame.fn.SourceMap[v.ip])
				if err == objects.ErrInvalidOperator {
					return fmt.Errorf("%s: invalid operation: %s &^ %s",
						filePos, (*left).TypeName(), (*right).TypeName())
				}

				return fmt.Errorf("%s: %s", filePos, err.Error())
			}

			if v.sp >= StackSize {
				return ErrStackOverflow
			}

			v.stack[v.sp] = &res
			v.sp++

		case compiler.OpBShiftLeft:
			right := v.stack[v.sp-1]
			left := v.stack[v.sp-2]
			v.sp -= 2

			res, err := (*left).BinaryOp(token.Shl, *right)
			if err != nil {
				filePos := v.fileSet.Position(v.curFrame.fn.SourceMap[v.ip])
				if err == objects.ErrInvalidOperator {
					return fmt.Errorf("%s: invalid operation: %s << %s",
						filePos, (*left).TypeName(), (*right).TypeName())
				}

				return fmt.Errorf("%s: %s", filePos, err.Error())
			}

			if v.sp >= StackSize {
				return ErrStackOverflow
			}

			v.stack[v.sp] = &res
			v.sp++

		case compiler.OpBShiftRight:
			right := v.stack[v.sp-1]
			left := v.stack[v.sp-2]
			v.sp -= 2

			res, err := (*left).BinaryOp(token.Shr, *right)
			if err != nil {
				filePos := v.fileSet.Position(v.curFrame.fn.SourceMap[v.ip])
				if err == objects.ErrInvalidOperator {
					return fmt.Errorf("%s: invalid operation: %s >> %s",
						filePos, (*left).TypeName(), (*right).TypeName())
				}

				return fmt.Errorf("%s: %s", filePos, err.Error())
			}

			if v.sp >= StackSize {
				return ErrStackOverflow
			}

			v.stack[v.sp] = &res
			v.sp++

		case compiler.OpEqual:
			right := v.stack[v.sp-1]
			left := v.stack[v.sp-2]
			v.sp -= 2

			if v.sp >= StackSize {
				return ErrStackOverflow
			}

			if (*left).Equals(*right) {
				v.stack[v.sp] = truePtr
			} else {
				v.stack[v.sp] = falsePtr
			}
			v.sp++

		case compiler.OpNotEqual:
			right := v.stack[v.sp-1]
			left := v.stack[v.sp-2]
			v.sp -= 2

			if v.sp >= StackSize {
				return ErrStackOverflow
			}

			if (*left).Equals(*right) {
				v.stack[v.sp] = falsePtr
			} else {
				v.stack[v.sp] = truePtr
			}
			v.sp++

		case compiler.OpGreaterThan:
			right := v.stack[v.sp-1]
			left := v.stack[v.sp-2]
			v.sp -= 2

			res, err := (*left).BinaryOp(token.Greater, *right)
			if err != nil {
				filePos := v.fileSet.Position(v.curFrame.fn.SourceMap[v.ip])
				if err == objects.ErrInvalidOperator {
					return fmt.Errorf("%s: invalid operation: %s > %s",
						filePos, (*left).TypeName(), (*right).TypeName())
				}

				return fmt.Errorf("%s: %s", filePos, err.Error())
			}

			if v.sp >= StackSize {
				return ErrStackOverflow
			}

			v.stack[v.sp] = &res
			v.sp++

		case compiler.OpGreaterThanEqual:
			right := v.stack[v.sp-1]
			left := v.stack[v.sp-2]
			v.sp -= 2

			res, err := (*left).BinaryOp(token.GreaterEq, *right)
			if err != nil {
				filePos := v.fileSet.Position(v.curFrame.fn.SourceMap[v.ip])
				if err == objects.ErrInvalidOperator {
					return fmt.Errorf("%s: invalid operation: %s >= %s",
						filePos, (*left).TypeName(), (*right).TypeName())
				}

				return fmt.Errorf("%s: %s", filePos, err.Error())
			}

			if v.sp >= StackSize {
				return ErrStackOverflow
			}

			v.stack[v.sp] = &res
			v.sp++

		case compiler.OpPop:
			v.sp--

		case compiler.OpTrue:
			if v.sp >= StackSize {
				return ErrStackOverflow
			}

			v.stack[v.sp] = truePtr
			v.sp++

		case compiler.OpFalse:
			if v.sp >= StackSize {
				return ErrStackOverflow
			}

			v.stack[v.sp] = falsePtr
			v.sp++

		case compiler.OpLNot:
			operand := v.stack[v.sp-1]
			v.sp--

			if v.sp >= StackSize {
				return ErrStackOverflow
			}

			if (*operand).IsFalsy() {
				v.stack[v.sp] = truePtr
			} else {
				v.stack[v.sp] = falsePtr
			}
			v.sp++

		case compiler.OpBComplement:
			operand := v.stack[v.sp-1]
			v.sp--

			switch x := (*operand).(type) {
			case *objects.Int:
				if v.sp >= StackSize {
					return ErrStackOverflow
				}

				var res objects.Object = &objects.Int{Value: ^x.Value}

				v.stack[v.sp] = &res
				v.sp++
			default:
				filePos := v.fileSet.Position(v.curFrame.fn.SourceMap[v.ip])
				return fmt.Errorf("%s: invalid operation: ^%s", filePos, (*operand).TypeName())
			}

		case compiler.OpMinus:
			operand := v.stack[v.sp-1]
			v.sp--

			switch x := (*operand).(type) {
			case *objects.Int:
				if v.sp >= StackSize {
					return ErrStackOverflow
				}

				var res objects.Object = &objects.Int{Value: -x.Value}

				v.stack[v.sp] = &res
				v.sp++
			case *objects.Float:
				if v.sp >= StackSize {
					return ErrStackOverflow
				}

				var res objects.Object = &objects.Float{Value: -x.Value}

				v.stack[v.sp] = &res
				v.sp++
			default:
				filePos := v.fileSet.Position(v.curFrame.fn.SourceMap[v.ip])
				return fmt.Errorf("%s: invalid operation: -%s", filePos, (*operand).TypeName())
			}

		case compiler.OpJumpFalsy:
			pos := int(v.curInsts[v.ip+2]) | int(v.curInsts[v.ip+1])<<8
			v.ip += 2

			condition := v.stack[v.sp-1]
			v.sp--

			if (*condition).IsFalsy() {
				v.ip = pos - 1
			}

		case compiler.OpAndJump:
			pos := int(v.curInsts[v.ip+2]) | int(v.curInsts[v.ip+1])<<8
			v.ip += 2

			condition := *v.stack[v.sp-1]
			if condition.IsFalsy() {
				v.ip = pos - 1
			} else {
				v.sp--
			}

		case compiler.OpOrJump:
			pos := int(v.curInsts[v.ip+2]) | int(v.curInsts[v.ip+1])<<8
			v.ip += 2

			condition := *v.stack[v.sp-1]
			if !condition.IsFalsy() {
				v.ip = pos - 1
			} else {
				v.sp--
			}

		case compiler.OpJump:
			pos := int(v.curInsts[v.ip+2]) | int(v.curInsts[v.ip+1])<<8
			v.ip = pos - 1

		case compiler.OpSetGlobal:
			globalIndex := int(v.curInsts[v.ip+2]) | int(v.curInsts[v.ip+1])<<8
			v.ip += 2

			v.sp--

			v.globals[globalIndex] = v.stack[v.sp]

		case compiler.OpSetSelGlobal:
			globalIndex := int(v.curInsts[v.ip+2]) | int(v.curInsts[v.ip+1])<<8
			numSelectors := int(v.curInsts[v.ip+3])
			v.ip += 3

			// selectors and RHS value
			selectors := v.stack[v.sp-numSelectors : v.sp]
			val := v.stack[v.sp-numSelectors-1]
			v.sp -= numSelectors + 1

			if err := indexAssign(v.globals[globalIndex], val, selectors); err != nil {
				filePos := v.fileSet.Position(v.curFrame.fn.SourceMap[v.ip-3])
				return fmt.Errorf("%s: %s", filePos, err.Error())
			}

		case compiler.OpGetGlobal:
			globalIndex := int(v.curInsts[v.ip+2]) | int(v.curInsts[v.ip+1])<<8
			v.ip += 2

			val := v.globals[globalIndex]

			if v.sp >= StackSize {
				return ErrStackOverflow
			}

			v.stack[v.sp] = val
			v.sp++

		case compiler.OpArray:
			numElements := int(v.curInsts[v.ip+2]) | int(v.curInsts[v.ip+1])<<8
			v.ip += 2

			var elements []objects.Object
			for i := v.sp - numElements; i < v.sp; i++ {
				elements = append(elements, *v.stack[i])
			}
			v.sp -= numElements

			var arr objects.Object = &objects.Array{Value: elements}

			if v.sp >= StackSize {
				return ErrStackOverflow
			}

			v.stack[v.sp] = &arr
			v.sp++

		case compiler.OpMap:
			numElements := int(v.curInsts[v.ip+2]) | int(v.curInsts[v.ip+1])<<8
			v.ip += 2

			kv := make(map[string]objects.Object)
			for i := v.sp - numElements; i < v.sp; i += 2 {
				key := *v.stack[i]
				value := *v.stack[i+1]
				kv[key.(*objects.String).Value] = value
			}
			v.sp -= numElements

			var m objects.Object = &objects.Map{Value: kv}

			if v.sp >= StackSize {
				return ErrStackOverflow
			}

			v.stack[v.sp] = &m
			v.sp++

		case compiler.OpError:
			value := v.stack[v.sp-1]

			var err objects.Object = &objects.Error{
				Value: *value,
			}

			v.stack[v.sp-1] = &err

		case compiler.OpImmutable:
			value := v.stack[v.sp-1]

			switch value := (*value).(type) {
			case *objects.Array:
				var immutableArray objects.Object = &objects.ImmutableArray{
					Value: value.Value,
				}
				v.stack[v.sp-1] = &immutableArray
			case *objects.Map:
				var immutableMap objects.Object = &objects.ImmutableMap{
					Value: value.Value,
				}
				v.stack[v.sp-1] = &immutableMap
			}

		case compiler.OpIndex:
			index := v.stack[v.sp-1]
			left := v.stack[v.sp-2]
			v.sp -= 2

			switch left := (*left).(type) {
			case objects.Indexable:
				val, err := left.IndexGet(*index)
				if err != nil {
					filePos := v.fileSet.Position(v.curFrame.fn.SourceMap[v.ip])

					if err == objects.ErrInvalidIndexType {
						return fmt.Errorf("%s: invalid index type: %s", filePos, (*index).TypeName())
					}

					return fmt.Errorf("%s: %s", filePos, err.Error())
				}
				if val == nil {
					val = objects.UndefinedValue
				}

				if v.sp >= StackSize {
					return ErrStackOverflow
				}

				v.stack[v.sp] = &val
				v.sp++

			case *objects.Error: // err.value
				key, ok := (*index).(*objects.String)
				if !ok || key.Value != "value" {
					filePos := v.fileSet.Position(v.curFrame.fn.SourceMap[v.ip])
					return fmt.Errorf("%s: invalid index on error", filePos)
				}

				if v.sp >= StackSize {
					return ErrStackOverflow
				}

				v.stack[v.sp] = &left.Value
				v.sp++

			default:
				filePos := v.fileSet.Position(v.curFrame.fn.SourceMap[v.ip])
				return fmt.Errorf("%s: not indexable: %s", filePos, left.TypeName())
			}

		case compiler.OpSliceIndex:
			high := v.stack[v.sp-1]
			low := v.stack[v.sp-2]
			left := v.stack[v.sp-3]
			v.sp -= 3

			var lowIdx int64
			if *low != objects.UndefinedValue {
				if low, ok := (*low).(*objects.Int); ok {
					lowIdx = low.Value
				} else {
					filePos := v.fileSet.Position(v.curFrame.fn.SourceMap[v.ip])
					return fmt.Errorf("%s: invalid slice index type: %s", filePos, low.TypeName())
				}
			}

			switch left := (*left).(type) {
			case *objects.Array:
				numElements := int64(len(left.Value))
				var highIdx int64
				if *high == objects.UndefinedValue {
					highIdx = numElements
				} else if high, ok := (*high).(*objects.Int); ok {
					highIdx = high.Value
				} else {
					filePos := v.fileSet.Position(v.curFrame.fn.SourceMap[v.ip])
					return fmt.Errorf("%s: invalid slice index type: %s", filePos, high.TypeName())
				}

				if lowIdx > highIdx {
					filePos := v.fileSet.Position(v.curFrame.fn.SourceMap[v.ip])
					return fmt.Errorf("%s: invalid slice index: %d > %d", filePos, lowIdx, highIdx)
				}

				if lowIdx < 0 {
					lowIdx = 0
				} else if lowIdx > numElements {
					lowIdx = numElements
				}

				if highIdx < 0 {
					highIdx = 0
				} else if highIdx > numElements {
					highIdx = numElements
				}

				if v.sp >= StackSize {
					return ErrStackOverflow
				}

				var val objects.Object = &objects.Array{Value: left.Value[lowIdx:highIdx]}
				v.stack[v.sp] = &val
				v.sp++

			case *objects.ImmutableArray:
				numElements := int64(len(left.Value))
				var highIdx int64
				if *high == objects.UndefinedValue {
					highIdx = numElements
				} else if high, ok := (*high).(*objects.Int); ok {
					highIdx = high.Value
				} else {
					filePos := v.fileSet.Position(v.curFrame.fn.SourceMap[v.ip])
					return fmt.Errorf("%s: invalid slice index type: %s", filePos, high.TypeName())
				}

				if lowIdx > highIdx {
					filePos := v.fileSet.Position(v.curFrame.fn.SourceMap[v.ip])
					return fmt.Errorf("%s: invalid slice index: %d > %d", filePos, lowIdx, highIdx)
				}

				if lowIdx < 0 {
					lowIdx = 0
				} else if lowIdx > numElements {
					lowIdx = numElements
				}

				if highIdx < 0 {
					highIdx = 0
				} else if highIdx > numElements {
					highIdx = numElements
				}

				if v.sp >= StackSize {
					return ErrStackOverflow
				}

				var val objects.Object = &objects.Array{Value: left.Value[lowIdx:highIdx]}

				v.stack[v.sp] = &val
				v.sp++

			case *objects.String:
				numElements := int64(len(left.Value))
				var highIdx int64
				if *high == objects.UndefinedValue {
					highIdx = numElements
				} else if high, ok := (*high).(*objects.Int); ok {
					highIdx = high.Value
				} else {
					filePos := v.fileSet.Position(v.curFrame.fn.SourceMap[v.ip])
					return fmt.Errorf("%s: invalid slice index type: %s", filePos, high.TypeName())
				}

				if lowIdx > highIdx {
					filePos := v.fileSet.Position(v.curFrame.fn.SourceMap[v.ip])
					return fmt.Errorf("%s: invalid slice index: %d > %d", filePos, lowIdx, highIdx)
				}

				if lowIdx < 0 {
					lowIdx = 0
				} else if lowIdx > numElements {
					lowIdx = numElements
				}

				if highIdx < 0 {
					highIdx = 0
				} else if highIdx > numElements {
					highIdx = numElements
				}

				if v.sp >= StackSize {
					return ErrStackOverflow
				}

				var val objects.Object = &objects.String{Value: left.Value[lowIdx:highIdx]}

				v.stack[v.sp] = &val
				v.sp++

			case *objects.Bytes:
				numElements := int64(len(left.Value))
				var highIdx int64
				if *high == objects.UndefinedValue {
					highIdx = numElements
				} else if high, ok := (*high).(*objects.Int); ok {
					highIdx = high.Value
				} else {
					filePos := v.fileSet.Position(v.curFrame.fn.SourceMap[v.ip])
					return fmt.Errorf("%s: invalid slice index type: %s", filePos, high.TypeName())
				}

				if lowIdx > highIdx {
					filePos := v.fileSet.Position(v.curFrame.fn.SourceMap[v.ip])
					return fmt.Errorf("%s: invalid slice index: %d > %d", filePos, lowIdx, highIdx)
				}

				if lowIdx < 0 {
					lowIdx = 0
				} else if lowIdx > numElements {
					lowIdx = numElements
				}

				if highIdx < 0 {
					highIdx = 0
				} else if highIdx > numElements {
					highIdx = numElements
				}

				if v.sp >= StackSize {
					return ErrStackOverflow
				}

				var val objects.Object = &objects.Bytes{Value: left.Value[lowIdx:highIdx]}

				v.stack[v.sp] = &val
				v.sp++
			}

		case compiler.OpCall:
			numArgs := int(v.curInsts[v.ip+1])
			v.ip++

			value := *v.stack[v.sp-1-numArgs]

			switch callee := value.(type) {
			case *objects.Closure:
				if numArgs != callee.Fn.NumParameters {
					filePos := v.fileSet.Position(v.curFrame.fn.SourceMap[v.ip-1])
					return fmt.Errorf("%s: wrong number of arguments: want=%d, got=%d",
						filePos, callee.Fn.NumParameters, numArgs)
				}

				// test if it's tail-call
				if callee.Fn == v.curFrame.fn { // recursion
					nextOp := v.curInsts[v.ip+1]
					if nextOp == compiler.OpReturnValue ||
						(nextOp == compiler.OpPop && compiler.OpReturn == v.curInsts[v.ip+2]) {
						for p := 0; p < numArgs; p++ {
							v.stack[v.curFrame.basePointer+p] = v.stack[v.sp-numArgs+p]
						}
						v.sp -= numArgs + 1
						v.ip = -1 // reset IP to beginning of the frame
						continue mainloop
					}
				}

				// update call frame
				v.curFrame.ip = v.ip // store current ip before call
				v.curFrame = &(v.frames[v.framesIndex])
				v.curFrame.fn = callee.Fn
				v.curFrame.freeVars = callee.Free
				v.curFrame.basePointer = v.sp - numArgs
				v.curInsts = callee.Fn.Instructions
				v.ip = -1
				v.curIPLimit = len(v.curInsts) - 1
				v.framesIndex++
				v.sp = v.sp - numArgs + callee.Fn.NumLocals

			case *objects.CompiledFunction:
				if numArgs != callee.NumParameters {
					filePos := v.fileSet.Position(v.curFrame.fn.SourceMap[v.ip-1])
					return fmt.Errorf("%s: wrong number of arguments: want=%d, got=%d",
						filePos, callee.NumParameters, numArgs)
				}

				// test if it's tail-call
				if callee == v.curFrame.fn { // recursion
					nextOp := v.curInsts[v.ip+1]
					if nextOp == compiler.OpReturnValue ||
						(nextOp == compiler.OpPop && compiler.OpReturn == v.curInsts[v.ip+2]) {
						for p := 0; p < numArgs; p++ {
							v.stack[v.curFrame.basePointer+p] = v.stack[v.sp-numArgs+p]
						}
						v.sp -= numArgs + 1
						v.ip = -1 // reset IP to beginning of the frame
						continue mainloop
					}
				}

				// update call frame
				v.curFrame.ip = v.ip // store current ip before call
				v.curFrame = &(v.frames[v.framesIndex])
				v.curFrame.fn = callee
				v.curFrame.freeVars = nil
				v.curFrame.basePointer = v.sp - numArgs
				v.curInsts = callee.Instructions
				v.ip = -1
				v.curIPLimit = len(v.curInsts) - 1
				v.framesIndex++
				v.sp = v.sp - numArgs + callee.NumLocals

			case objects.Callable:
				var args []objects.Object
				for _, arg := range v.stack[v.sp-numArgs : v.sp] {
					args = append(args, *arg)
				}

				ret, err := callee.Call(args...)
				v.sp -= numArgs + 1

				// runtime error
				if err != nil {
					filePos := v.fileSet.Position(v.curFrame.fn.SourceMap[v.ip-1])

					if err == objects.ErrWrongNumArguments {
						return fmt.Errorf("%s: wrong number of arguments in call to '%s'",
							filePos, value.TypeName())
					}

					if err, ok := err.(objects.ErrInvalidArgumentType); ok {
						return fmt.Errorf("%s: invalid type for argument '%s' in call to '%s': expected %s, found %s",
							filePos, err.Name, value.TypeName(), err.Expected, err.Found)
					}

					return fmt.Errorf("%s: %s", filePos, err.Error())
				}

				// nil return -> undefined
				if ret == nil {
					ret = objects.UndefinedValue
				}

				if v.sp >= StackSize {
					return ErrStackOverflow
				}

				v.stack[v.sp] = &ret
				v.sp++

			default:
				filePos := v.fileSet.Position(v.curFrame.fn.SourceMap[v.ip-1])
				return fmt.Errorf("%s: not callable: %s", filePos, callee.TypeName())
			}

		case compiler.OpReturnValue:
			retVal := v.stack[v.sp-1]
			//v.sp--

			v.framesIndex--
			lastFrame := v.frames[v.framesIndex]
			v.curFrame = &v.frames[v.framesIndex-1]
			v.curInsts = v.curFrame.fn.Instructions
			v.curIPLimit = len(v.curInsts) - 1
			v.ip = v.curFrame.ip

			//v.sp = lastFrame.basePointer - 1
			v.sp = lastFrame.basePointer

			// skip stack overflow check because (newSP) <= (oldSP)
			//if v.sp-1 >= StackSize {
			//	return ErrStackOverflow
			//}

			v.stack[v.sp-1] = retVal
			//v.sp++

		case compiler.OpReturn:
			v.framesIndex--
			lastFrame := v.frames[v.framesIndex]
			v.curFrame = &v.frames[v.framesIndex-1]
			v.curInsts = v.curFrame.fn.Instructions
			v.curIPLimit = len(v.curInsts) - 1
			v.ip = v.curFrame.ip

			//v.sp = lastFrame.basePointer - 1
			v.sp = lastFrame.basePointer

			// skip stack overflow check because (newSP) <= (oldSP)
			//if v.sp-1 >= StackSize {
			//	return ErrStackOverflow
			//}

			v.stack[v.sp-1] = undefinedPtr
			//v.sp++

		case compiler.OpDefineLocal:
			localIndex := int(v.curInsts[v.ip+1])
			v.ip++

			sp := v.curFrame.basePointer + localIndex

			// local variables can be mutated by other actions
			// so always store the copy of popped value
			val := *v.stack[v.sp-1]
			v.sp--

			v.stack[sp] = &val

		case compiler.OpSetLocal:
			localIndex := int(v.curInsts[v.ip+1])
			v.ip++

			sp := v.curFrame.basePointer + localIndex

			// update pointee of v.stack[sp] instead of replacing the pointer itself.
			// this is needed because there can be free variables referencing the same local variables.
			val := v.stack[v.sp-1]
			v.sp--

			*v.stack[sp] = *val // also use a copy of popped value

		case compiler.OpSetSelLocal:
			localIndex := int(v.curInsts[v.ip+1])
			numSelectors := int(v.curInsts[v.ip+2])
			v.ip += 2

			// selectors and RHS value
			selectors := v.stack[v.sp-numSelectors : v.sp]
			val := v.stack[v.sp-numSelectors-1]
			v.sp -= numSelectors + 1

			sp := v.curFrame.basePointer + localIndex

			if err := indexAssign(v.stack[sp], val, selectors); err != nil {
				filePos := v.fileSet.Position(v.curFrame.fn.SourceMap[v.ip-2])
				return fmt.Errorf("%s: %s", filePos, err.Error())
			}

		case compiler.OpGetLocal:
			localIndex := int(v.curInsts[v.ip+1])
			v.ip++

			val := v.stack[v.curFrame.basePointer+localIndex]

			if v.sp >= StackSize {
				return ErrStackOverflow
			}

			v.stack[v.sp] = val
			v.sp++

		case compiler.OpGetBuiltin:
			builtinIndex := int(v.curInsts[v.ip+1])
			v.ip++

			if v.sp >= StackSize {
				return ErrStackOverflow
			}

			v.stack[v.sp] = &builtinFuncs[builtinIndex]
			v.sp++

		case compiler.OpGetBuiltinModule:
			val := v.stack[v.sp-1]
			v.sp--

			moduleName := (*val).(*objects.String).Value

			module, ok := v.builtinModules[moduleName]
			if !ok {
				filePos := v.fileSet.Position(v.curFrame.fn.SourceMap[v.ip-3])
				return fmt.Errorf("%s: module '%s' not found", filePos, moduleName)
			}

			if v.sp >= StackSize {
				return ErrStackOverflow
			}

			v.stack[v.sp] = module
			v.sp++

		case compiler.OpClosure:
			constIndex := int(v.curInsts[v.ip+2]) | int(v.curInsts[v.ip+1])<<8
			numFree := int(v.curInsts[v.ip+3])
			v.ip += 3

			fn, ok := v.constants[constIndex].(*objects.CompiledFunction)
			if !ok {
				filePos := v.fileSet.Position(v.curFrame.fn.SourceMap[v.ip-3])
				return fmt.Errorf("%s: not function: %s", filePos, fn.TypeName())
			}

			free := make([]*objects.Object, numFree)
			for i := 0; i < numFree; i++ {
				free[i] = v.stack[v.sp-numFree+i]
			}
			v.sp -= numFree

			if v.sp >= StackSize {
				return ErrStackOverflow
			}

			var cl objects.Object = &objects.Closure{
				Fn:   fn,
				Free: free,
			}

			v.stack[v.sp] = &cl
			v.sp++

		case compiler.OpGetFree:
			freeIndex := int(v.curInsts[v.ip+1])
			v.ip++

			val := v.curFrame.freeVars[freeIndex]

			if v.sp >= StackSize {
				return ErrStackOverflow
			}

			v.stack[v.sp] = val
			v.sp++

		case compiler.OpSetSelFree:
			freeIndex := int(v.curInsts[v.ip+1])
			numSelectors := int(v.curInsts[v.ip+2])
			v.ip += 2

			// selectors and RHS value
			selectors := v.stack[v.sp-numSelectors : v.sp]
			val := v.stack[v.sp-numSelectors-1]
			v.sp -= numSelectors + 1

			if err := indexAssign(v.curFrame.freeVars[freeIndex], val, selectors); err != nil {
				filePos := v.fileSet.Position(v.curFrame.fn.SourceMap[v.ip-2])
				return fmt.Errorf("%s: %s", filePos, err.Error())
			}

		case compiler.OpSetFree:
			freeIndex := int(v.curInsts[v.ip+1])
			v.ip++

			val := v.stack[v.sp-1]
			v.sp--

			*v.curFrame.freeVars[freeIndex] = *val

		case compiler.OpIteratorInit:
			var iterator objects.Object

			dst := v.stack[v.sp-1]
			v.sp--

			iterable, ok := (*dst).(objects.Iterable)
			if !ok {
				filePos := v.fileSet.Position(v.curFrame.fn.SourceMap[v.ip])
				return fmt.Errorf("%s: not iterable: %s", filePos, (*dst).TypeName())
			}

			iterator = iterable.Iterate()

			if v.sp >= StackSize {
				return ErrStackOverflow
			}

			v.stack[v.sp] = &iterator
			v.sp++

		case compiler.OpIteratorNext:
			iterator := v.stack[v.sp-1]
			v.sp--

			hasMore := (*iterator).(objects.Iterator).Next()

			if v.sp >= StackSize {
				return ErrStackOverflow
			}

			if hasMore {
				v.stack[v.sp] = truePtr
			} else {
				v.stack[v.sp] = falsePtr
			}
			v.sp++

		case compiler.OpIteratorKey:
			iterator := v.stack[v.sp-1]
			v.sp--

			val := (*iterator).(objects.Iterator).Key()

			if v.sp >= StackSize {
				return ErrStackOverflow
			}

			v.stack[v.sp] = &val
			v.sp++

		case compiler.OpIteratorValue:
			iterator := v.stack[v.sp-1]
			v.sp--

			val := (*iterator).(objects.Iterator).Value()

			if v.sp >= StackSize {
				return ErrStackOverflow
			}

			v.stack[v.sp] = &val
			v.sp++

		default:
			panic(fmt.Errorf("unknown opcode: %d", v.curInsts[v.ip]))
		}
	}

	// check if stack still has some objects left
	if v.sp > 0 && atomic.LoadInt64(&v.aborting) == 0 {
		panic(fmt.Errorf("non empty stack after execution: %d", v.sp))
	}

	return nil
}

// Globals returns the global variables.
func (v *VM) Globals() []*objects.Object {
	return v.globals
}

// FrameInfo returns the current function call frame information.
func (v *VM) FrameInfo() (frameIndex, ip int) {
	return v.framesIndex - 1, v.ip
}

func indexAssign(dst, src *objects.Object, selectors []*objects.Object) error {
	numSel := len(selectors)

	for sidx := numSel - 1; sidx > 0; sidx-- {
		indexable, ok := (*dst).(objects.Indexable)
		if !ok {
			return fmt.Errorf("not indexable: %s", (*dst).TypeName())
		}

		next, err := indexable.IndexGet(*selectors[sidx])
		if err != nil {
			if err == objects.ErrInvalidIndexType {
				return fmt.Errorf("invalid index type: %s", (*selectors[sidx]).TypeName())
			}

			return err
		}

		dst = &next
	}

	indexAssignable, ok := (*dst).(objects.IndexAssignable)
	if !ok {
		return fmt.Errorf("not index-assignable: %s", (*dst).TypeName())
	}

	if err := indexAssignable.IndexSet(*selectors[0], *src); err != nil {
		if err == objects.ErrInvalidIndexValueType {
			return fmt.Errorf("invaid index value type: %s", (*src).TypeName())
		}

		return err
	}

	return nil
}

func init() {
	builtinFuncs = make([]objects.Object, len(objects.Builtins))
	for i, b := range objects.Builtins {
		builtinFuncs[i] = &objects.BuiltinFunction{
			Name:  b.Name,
			Value: b.Func,
		}
	}
}
