// cmd/7l/asm.c, cmd/7l/asmout.c, cmd/7l/optab.c, cmd/7l/span.c, cmd/ld/sub.c, cmd/ld/mod.c, from Vita Nuova.
// https://code.google.com/p/ken-cc/source/browse/
//
// 	Copyright © 1994-1999 Lucent Technologies Inc.  All rights reserved.
// 	Portions Copyright © 1995-1997 C H Forsyth (forsyth@terzarima.net)
// 	Portions Copyright © 1997-1999 Vita Nuova Limited
// 	Portions Copyright © 2000-2007 Vita Nuova Holdings Limited (www.vitanuova.com)
// 	Portions Copyright © 2004,2006 Bruce Ellis
// 	Portions Copyright © 2005-2007 C H Forsyth (forsyth@terzarima.net)
// 	Revisions Copyright © 2000-2007 Lucent Technologies Inc. and others
// 	Portions Copyright © 2009 The Go Authors.  All rights reserved.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.  IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package arm64

import (
	"cmd/internal/obj"
	"fmt"
	"log"
	"math"
	"sort"
)

const (
	FuncAlign = 16
)

const (
	REGFROM = 1
)

type Optab struct {
	as    uint16
	a1    uint8
	a2    uint8
	a3    uint8
	type_ int8
	size  int8
	param int8
	flag  int8
}

type Oprang struct {
	start []Optab
	stop  []Optab
}

type Opcross [32][2][32]uint8

var oprange [ALAST]Oprang

var opcross [8]Opcross

var repop [ALAST]uint8

var xcmp [C_NCLASS][C_NCLASS]uint8

const (
	S32     = 0 << 31
	S64     = 1 << 31
	Sbit    = 1 << 29
	LSL0_32 = 2 << 13
	LSL0_64 = 3 << 13
)

func OPDP2(x uint32) uint32 {
	return 0<<30 | 0<<29 | 0xd6<<21 | x<<10
}

func OPDP3(sf uint32, op54 uint32, op31 uint32, o0 uint32) uint32 {
	return sf<<31 | op54<<29 | 0x1B<<24 | op31<<21 | o0<<15
}

func OPBcc(x uint32) uint32 {
	return 0x2A<<25 | 0<<24 | 0<<4 | x&15
}

func OPBLR(x uint32) uint32 {
	/* x=0, JMP; 1, CALL; 2, RET */
	return 0x6B<<25 | 0<<23 | x<<21 | 0x1F<<16 | 0<<10
}

func SYSOP(l uint32, op0 uint32, op1 uint32, crn uint32, crm uint32, op2 uint32, rt uint32) uint32 {
	return 0x354<<22 | l<<21 | op0<<19 | op1<<16 | crn<<12 | crm<<8 | op2<<5 | rt
}

func SYSHINT(x uint32) uint32 {
	return SYSOP(0, 0, 3, 2, 0, x, 0x1F)
}

func LDSTR12U(sz uint32, v uint32, opc uint32) uint32 {
	return sz<<30 | 7<<27 | v<<26 | 1<<24 | opc<<22
}

func LDSTR9S(sz uint32, v uint32, opc uint32) uint32 {
	return sz<<30 | 7<<27 | v<<26 | 0<<24 | opc<<22
}

func LD2STR(o uint32) uint32 {
	return o &^ (3 << 22)
}

func LDSTX(sz uint32, o2 uint32, l uint32, o1 uint32, o0 uint32) uint32 {
	return sz<<30 | 0x8<<24 | o2<<23 | l<<22 | o1<<21 | o0<<15
}

func FPCMP(m uint32, s uint32, type_ uint32, op uint32, op2 uint32) uint32 {
	return m<<31 | s<<29 | 0x1E<<24 | type_<<22 | 1<<21 | op<<14 | 8<<10 | op2
}

func FPCCMP(m uint32, s uint32, type_ uint32, op uint32) uint32 {
	return m<<31 | s<<29 | 0x1E<<24 | type_<<22 | 1<<21 | 1<<10 | op<<4
}

func FPOP1S(m uint32, s uint32, type_ uint32, op uint32) uint32 {
	return m<<31 | s<<29 | 0x1E<<24 | type_<<22 | 1<<21 | op<<15 | 0x10<<10
}

func FPOP2S(m uint32, s uint32, type_ uint32, op uint32) uint32 {
	return m<<31 | s<<29 | 0x1E<<24 | type_<<22 | 1<<21 | op<<12 | 2<<10
}

func FPCVTI(sf uint32, s uint32, type_ uint32, rmode uint32, op uint32) uint32 {
	return sf<<31 | s<<29 | 0x1E<<24 | type_<<22 | 1<<21 | rmode<<19 | op<<16 | 0<<10
}

func ADR(p uint32, o uint32, rt uint32) uint32 {
	return p<<31 | (o&3)<<29 | 0x10<<24 | ((o>>2)&0x7FFFF)<<5 | rt
}

func OPBIT(x uint32) uint32 {
	return 1<<30 | 0<<29 | 0xD6<<21 | 0<<16 | x<<10
}

const (
	LFROM = 1 << 0
	LTO   = 1 << 1
	LPOOL = 1 << 2
)

var optab = []Optab{}

/*
 * internal class codes for different constant classes:
 * they partition the constant/offset range into disjoint ranges that
 * are somehow treated specially by one or more load/store instructions.
 */
var autoclass = []int{C_PSAUTO, C_NSAUTO, C_NPAUTO, C_PSAUTO, C_PPAUTO, C_UAUTO4K, C_UAUTO8K, C_UAUTO16K, C_UAUTO32K, C_UAUTO64K, C_LAUTO}

var oregclass = []int{C_ZOREG, C_NSOREG, C_NPOREG, C_PSOREG, C_PPOREG, C_UOREG4K, C_UOREG8K, C_UOREG16K, C_UOREG32K, C_UOREG64K, C_LOREG}

/*
 * valid pstate field values, and value to use in instruction
 */
var pstatefield = []struct {
	a uint32
	b uint32
}{
	struct {
		a uint32
		b uint32
	}{D_SPSel, 0<<16 | 4<<12 | 5<<5},
	struct {
		a uint32
		b uint32
	}{D_DAIFSet, 3<<16 | 4<<12 | 6<<5},
	struct {
		a uint32
		b uint32
	}{D_DAIFClr, 3<<16 | 4<<12 | 7<<5},
}

var pool struct {
	start uint32
	size  uint32
}

func prasm(p *obj.Prog) {
	fmt.Printf("%v\n", p)
}

func span7(ctxt *obj.Link, cursym *obj.LSym) {
	var p *obj.Prog
	var o *Optab
	var m int
	var bflag int
	var i int
	var c int32
	var psz int32
	var out [6]uint32
	var bp []byte

	p = cursym.Text
	if p == nil || p.Link == nil { // handle external functions and ELF section symbols
		return
	}
	ctxt.Cursym = cursym
	ctxt.Autosize = int32(p.To.Offset&0xffffffff) + 8

	if oprange[AAND].start == nil {
		buildop(ctxt)
	}

	bflag = 0
	c = 0
	p.Pc = int64(c)
	for p = p.Link; p != nil; p = p.Link {
		ctxt.Curp = p
		if p.As == ADWORD && (c&7) != 0 {
			c += 4
		}
		p.Pc = int64(c)
		o = oplook(ctxt, p)
		m = int(o.size)
		if m == 0 {
			if p.As != ANOP && p.As != AFUNCDATA && p.As != APCDATA {
				ctxt.Diag("zero-width instruction\n%v", p)
			}
			continue
		}

		switch o.flag & (LFROM | LTO) {
		case LFROM:
			addpool(ctxt, p, &p.From)

		case LTO:
			addpool(ctxt, p, &p.To)
			break
		}

		if p.As == AB || p.As == ARET || p.As == AERET || p.As == ARETURN { /* TO DO: other unconditional operations */
			checkpool(ctxt, p, 0)
		}
		c += int32(m)
		if ctxt.Blitrl != nil {
			checkpool(ctxt, p, 1)
		}
	}

	cursym.Size = int64(c)

	/*
	 * if any procedure is large enough to
	 * generate a large SBRA branch, then
	 * generate extra passes putting branches
	 * around jmps to fix. this is rare.
	 */
	for bflag != 0 {

		if ctxt.Debugvlog != 0 {
			fmt.Fprintf(ctxt.Bso, "%5.2f span1\n", obj.Cputime())
		}
		bflag = 0
		c = 0
		for p = cursym.Text; p != nil; p = p.Link {
			if p.As == ADWORD && (c&7) != 0 {
				c += 4
			}
			p.Pc = int64(c)
			o = oplook(ctxt, p)

			/* very large branches
			if(o->type == 6 && p->cond) {
				otxt = p->cond->pc - c;
				if(otxt < 0)
					otxt = -otxt;
				if(otxt >= (1L<<17) - 10) {
					q = ctxt->arch->prg();
					q->link = p->link;
					p->link = q;
					q->as = AB;
					q->to.type = obj.TYPE_BRANCH;
					q->cond = p->cond;
					p->cond = q;
					q = ctxt->arch->prg();
					q->link = p->link;
					p->link = q;
					q->as = AB;
					q->to.type = obj.TYPE_BRANCH;
					q->cond = q->link->link;
					bflag = 1;
				}
			}
			*/
			m = int(o.size)

			if m == 0 {
				if p.As != ANOP && p.As != AFUNCDATA && p.As != APCDATA {
					ctxt.Diag("zero-width instruction\n%v", p)
				}
				continue
			}

			c += int32(m)
		}
	}

	c += -c & (FuncAlign - 1)
	cursym.Size = int64(c)

	/*
	 * lay out the code, emitting code and data relocations.
	 */
	if ctxt.Tlsg == nil {

		ctxt.Tlsg = obj.Linklookup(ctxt, "runtime.tlsg", 0)
	}
	obj.Symgrow(ctxt, cursym, cursym.Size)
	bp = cursym.P
	psz = 0
	for p = cursym.Text.Link; p != nil; p = p.Link {
		ctxt.Pc = p.Pc
		ctxt.Curp = p
		o = oplook(ctxt, p)

		// need to align DWORDs on 8-byte boundary. The ISA doesn't
		// require it, but the various 64-bit loads we generate assume it.
		if o.as == ADWORD && psz%8 != 0 {

			bp[3] = 0
			bp[2] = bp[3]
			bp[1] = bp[2]
			bp[0] = bp[1]
			bp = bp[4:]
			psz += 4
		}

		if int(o.size) > 4*len(out) {
			log.Fatalf("out array in span7 is too small, need at least %d for %v", o.size/4, p)
		}
		asmout(ctxt, p, o, out[:])
		for i = 0; i < int(o.size/4); i++ {
			ctxt.Arch.ByteOrder.PutUint32(bp, out[i])
			bp = bp[4:]
			psz += 4
		}
	}
}

/*
 * when the first reference to the literal pool threatens
 * to go out of range of a 1Mb PC-relative offset
 * drop the pool now, and branch round it.
 */
func checkpool(ctxt *obj.Link, p *obj.Prog, skip int) {

	if pool.size >= 0xffff0 || !(ispcdisp(int32(p.Pc+4+int64(pool.size)-int64(pool.start)+8)) != 0) {
		flushpool(ctxt, p, skip)
	} else if p.Link == nil {
		flushpool(ctxt, p, 2)
	}
}

func flushpool(ctxt *obj.Link, p *obj.Prog, skip int) {
	var q *obj.Prog
	if ctxt.Blitrl != nil {
		if skip != 0 {
			if ctxt.Debugvlog != 0 && skip == 1 {
				fmt.Printf("note: flush literal pool at %#x: len=%d ref=%x\n", uint64(p.Pc+4), pool.size, pool.start)
			}
			q = ctxt.NewProg()
			q.As = AB
			q.To.Type = obj.TYPE_BRANCH
			q.Pcond = p.Link
			q.Link = ctxt.Blitrl
			q.Lineno = p.Lineno
			ctxt.Blitrl = q
		} else if p.Pc+int64(pool.size)-int64(pool.start) < 1024*1024 {
			return
		}
		ctxt.Elitrl.Link = p.Link
		p.Link = ctxt.Blitrl

		// BUG(minux): how to correctly handle line number for constant pool entries?
		// for now, we set line number to the last instruction preceding them at least
		// this won't bloat the .debug_line tables
		for ctxt.Blitrl != nil {

			ctxt.Blitrl.Lineno = p.Lineno
			ctxt.Blitrl = ctxt.Blitrl.Link
		}

		ctxt.Blitrl = nil /* BUG: should refer back to values until out-of-range */
		ctxt.Elitrl = nil
		pool.size = 0
		pool.start = 0
	}
}

/*
 * TO DO: hash
 */
func addpool(ctxt *obj.Link, p *obj.Prog, a *obj.Addr) {

	var q *obj.Prog
	var t obj.Prog
	var c int
	var sz int
	c = aclass(ctxt, a)
	t = *ctxt.NewProg()
	t.As = AWORD
	sz = 4

	// MOVW foo(SB), R is actually
	//	MOV addr, REGTEMP
	//	MOVW REGTEMP, R
	// where addr is the address of the DWORD containing the address of foo.
	if p.As == AMOV || c == C_ADDR || c == C_VCON {

		t.As = ADWORD
		sz = 8
	}

	switch c {
	// TODO(aram): remove.
	default:
		if a.Name != D_EXTERN {

			fmt.Printf("addpool: %v in %v shouldn't go to default case\n", DRconv(c), p)
		}

		t.To.Offset = a.Offset
		t.To.Sym = a.Sym
		t.To.Type = a.Type
		t.To.Name = a.Name

		/* This is here to work around a bug where we generate negative
		operands that match C_MOVCON, but we use them with
		instructions that only accept unsigned immediates. This
		will cause oplook to return a variant of the instruction
		that loads the negative constant from memory, rather than
		using the immediate form. Because of that load, we get here,
		so we need to know what to do with C_MOVCON.

		The correct fix is to use the "negation" instruction variant,
		e.g. CMN $1, R instead of CMP $-1, R, or SUB $1, R instead
		of ADD $-1, R. */
	case C_MOVCON,

		/* This is here because MOV uint12<<12, R is disabled in optab.
		Because of this, we need to load the constant from memory. */
		C_ADDCON,

		/* These are here because they are disabled in optab.
		Because of this, we need to load the constant from memory. */
		C_BITCON,
		C_ABCON,
		C_PSAUTO,
		C_PPAUTO,
		C_UAUTO4K,
		C_UAUTO8K,
		C_UAUTO16K,
		C_UAUTO32K,
		C_UAUTO64K,
		C_NSAUTO,
		C_NPAUTO,
		C_LAUTO,
		C_PPOREG,
		C_PSOREG,
		C_UOREG4K,
		C_UOREG8K,
		C_UOREG16K,
		C_UOREG32K,
		C_UOREG64K,
		C_NSOREG,
		C_NPOREG,
		C_LOREG,
		C_LACON,
		C_LCON,
		C_VCON:
		if a.Name == D_EXTERN {

			fmt.Printf("addpool: %v in %v needs reloc\n", DRconv(c), p)
		}

		t.To.Type = D_CONST
		t.To.Offset = ctxt.Instoffset
		break
	}

	for q = ctxt.Blitrl; q != nil; q = q.Link { /* could hash on t.t0.offset */
		if q.To == t.To {
			p.Pcond = q
			return
		}
	}

	q = ctxt.NewProg()
	*q = t
	q.Pc = int64(pool.size)
	if ctxt.Blitrl == nil {
		ctxt.Blitrl = q
		pool.start = uint32(p.Pc)
	} else {

		ctxt.Elitrl.Link = q
	}
	ctxt.Elitrl = q
	pool.size = -pool.size & (FuncAlign - 1)
	pool.size += uint32(sz)
	p.Pcond = q
}

func regoff(ctxt *obj.Link, a *obj.Addr) uint32 {
	ctxt.Instoffset = 0
	aclass(ctxt, a)
	return uint32(ctxt.Instoffset)
}

func ispcdisp(v int32) int {
	/* pc-relative addressing will reach? */
	return bool2int(v >= -0xfffff && v <= 0xfffff && (v&3) == 0)
}

func isaddcon(v int64) int {
	/* uimm12 or uimm24? */
	if v < 0 {

		return 0
	}
	if (v & 0xFFF) == 0 {
		v >>= 12
	}
	return bool2int(v <= 0xFFF)
}

func isbitcon(v uint64) int {
	/*  fancy bimm32 or bimm64? */
	return bool2int(findmask(v) != nil || (v>>32) == 0 && findmask(v|(v<<32)) != nil)
}

/*
 * return appropriate index into tables above
 */
func constclass(l int64) int {

	if l == 0 {
		return 0
	}
	if l < 0 {
		if l >= -256 {
			return 1
		}
		if l >= -512 && (l&7) == 0 {
			return 2
		}
		return 10
	}

	if l <= 255 {
		return 3
	}
	if l <= 504 && (l&7) == 0 {
		return 4
	}
	if l <= 4095 {
		return 5
	}
	if l <= 8190 && (l&1) == 0 {
		return 6
	}
	if l <= 16380 && (l&3) == 0 {
		return 7
	}
	if l <= 32760 && (l&7) == 0 {
		return 8
	}
	if l <= 65520 && (l&0xF) == 0 {
		return 9
	}
	return 10
}

/*
 * given an offset v and a class c (see above)
 * return the offset value to use in the instruction,
 * scaled if necessary
 */
func offsetshift(ctxt *obj.Link, v int64, c int) int64 {

	var vs int64
	var s int
	var shifts = []int{0, 1, 2, 3, 4}
	s = 0
	if c >= C_SEXT1 && c <= C_SEXT16 {
		s = shifts[c-C_SEXT1]
	} else if c >= C_UAUTO4K && c <= C_UAUTO64K {
		s = shifts[c-C_UAUTO4K]
	} else if c >= C_UOREG4K && c <= C_UOREG64K {
		s = shifts[c-C_UOREG4K]
	}
	vs = v >> uint(s)
	if vs<<uint(s) != v {
		ctxt.Diag("odd offset: %d\n%v", v, ctxt.Curp)
	}
	return vs
}

/*
 * if v contains a single 16-bit value aligned
 * on a 16-bit field, and thus suitable for movk/movn,
 * return the field index 0 to 3; otherwise return -1
 */
func movcon(v int64) int {

	var s int
	for s = 0; s < 64; s += 16 {
		if (uint64(v) &^ (uint64(0xFFFF) << uint(s))) == 0 {
			return s / 16
		}
	}
	return -1
}

func aclass(ctxt *obj.Link, a *obj.Addr) int {
	switch a.Type {
	case obj.TYPE_NONE:
		return C_NONE

	case obj.TYPE_REG:
		switch {
		case a.Reg == REGSP:
			return C_RSP
		case REG_R0 <= a.Reg && a.Reg <= REG_R31:
			return C_REG
		case REG_F0 <= a.Reg && a.Reg <= REG_F31:
			return C_FREG
		case REG_V0 <= a.Reg && a.Reg <= REG_V31:
			return C_VREG
		case a.Reg&REG_EXT != 0:
			return C_EXTREG
		case a.Reg >= REG_SPECIAL:
			return C_SPR
		}
		return C_GOK

	case obj.TYPE_REGREG:
		return C_PAIR

	case obj.TYPE_SHIFT:
		return C_SHIFT

	case obj.TYPE_MEM:
		switch a.Name {
		case obj.NAME_EXTERN,
			obj.NAME_STATIC:
			if a.Sym == nil {
				break
			}
			ctxt.Instoffset = a.Offset
			if a.Sym != nil { // use relocation
				return C_ADDR
			}
			return C_LEXT

		case obj.NAME_AUTO:
			ctxt.Instoffset = int64(ctxt.Autosize) + a.Offset
			return autoclass[constclass(ctxt.Instoffset)]

		case obj.NAME_PARAM:
			ctxt.Instoffset = int64(ctxt.Autosize) + a.Offset + 8
			return autoclass[constclass(ctxt.Instoffset)]

		case obj.TYPE_NONE:
			ctxt.Instoffset = a.Offset
			return oregclass[constclass(ctxt.Instoffset)]
		}
		return C_GOK

	case obj.TYPE_FCONST:
		return C_FCON

	case obj.TYPE_TEXTSIZE:
		return C_TEXTSIZE

	case obj.TYPE_CONST,
		obj.TYPE_ADDR:
		switch a.Name {
		case obj.TYPE_NONE:
			ctxt.Instoffset = a.Offset
			if a.Reg == REGSP {
				goto aconsize
			}
			v := ctxt.Instoffset
			if v == 0 {
				return C_ZCON
			}
			if isaddcon(v) != 0 {
				if v <= 0xFFF {
					return C_ADDCON0
				}
				if isbitcon(uint64(v)) != 0 {
					return C_ABCON
				}
				return C_ADDCON
			}

			t := movcon(v)
			if t >= 0 {
				if isbitcon(uint64(v)) != 0 {
					return C_MBCON
				}
				return C_MOVCON
			}

			t = movcon(^v)
			if t >= 0 {
				if isbitcon(uint64(v)) != 0 {
					return C_MBCON
				}
				return C_MOVCON
			}

			if isbitcon(uint64(v)) != 0 {
				return C_BITCON
			}
			return C_VCON

		case obj.NAME_EXTERN,
			obj.NAME_STATIC:
			s := a.Sym
			if s == nil {
				break
			}
			ctxt.Instoffset = a.Offset
			return C_VCONADDR

		case obj.NAME_AUTO:
			ctxt.Instoffset = int64(ctxt.Autosize) + a.Offset
			goto aconsize

		case obj.NAME_PARAM:
			ctxt.Instoffset = int64(ctxt.Autosize) + a.Offset + 8
			goto aconsize
		}
		return C_GOK

	aconsize:
		if isaddcon(ctxt.Instoffset) != 0 {
			return C_AACON
		}
		return C_LACON

	case obj.TYPE_BRANCH:
		return C_SBRA
	}

	return C_GOK

}

func oplook(ctxt *obj.Link, p *obj.Prog) *Optab {
	var a1 int
	var a2 int
	var a3 int
	var r int
	var c1 []byte
	var c2 []byte
	var c3 []byte
	var o []Optab
	var e []Optab
	a1 = int(p.Optab)
	if a1 != 0 {
		return &optab[a1-1:][0]
	}
	a1 = int(p.From.Class)
	if a1 == 0 {
		a1 = aclass(ctxt, &p.From) + 1
		p.From.Class = int8(a1)
	}

	a1--
	a3 = int(p.To.Class)
	if a3 == 0 {
		a3 = aclass(ctxt, &p.To) + 1
		p.To.Class = int8(a3)
	}

	a3--
	a2 = C_NONE
	if p.Reg != NREG {
		a2 = C_REG
	}
	r = int(p.As)
	o = oprange[r].start
	if o == nil {
		a1 = int(opcross[repop[r]][a1][a2][a3])
		if a1 != 0 {
			p.Optab = uint16(a1 + 1)
			return &optab[a1:][0]
		}

		o = oprange[r].stop /* just generate an error */
	}

	if false {
		fmt.Printf("oplook %v %d %d %d\n", Aconv(int(p.As)), a1, a2, a3)
		fmt.Printf("\t\t%d %d\n", p.From.Type, p.To.Type)
	}

	e = oprange[r].stop
	c1 = xcmp[a1][:]
	c2 = xcmp[a2][:]
	c3 = xcmp[a3][:]
	for ; -cap(o) < -cap(e); o = o[1:] {
		if int(o[0].a2) == a2 || c2[o[0].a2] != 0 {
			if c1[o[0].a1] != 0 {
				if c3[o[0].a3] != 0 {
					p.Optab = uint16((-cap(o) + cap(optab)) + 1)
					return &o[0]
				}
			}
		}
	}

	ctxt.Diag("illegal combination %v %v %v %v, %d %d", p, DRconv(a1), DRconv(a2), DRconv(a3), p.From.Type, p.To.Type)
	prasm(p)
	if o == nil {
		o = optab
	}
	return &o[0]
}

func cmp(a int, b int) bool {
	if a == b {
		return true
	}
	switch a {
	case C_RSP:
		if b == C_REG {
			return true
		}

	case C_REG:
		if b == C_ZCON {
			return true
		}

	case C_ADDCON0:
		if b == C_ZCON {
			return true
		}

	case C_ADDCON:
		if b == C_ZCON || b == C_ADDCON0 || b == C_ABCON {
			return true
		}

	case C_BITCON:
		if b == C_ABCON || b == C_MBCON {
			return true
		}

	case C_MOVCON:
		if b == C_MBCON || b == C_ZCON || b == C_ADDCON0 {
			return true
		}

	case C_LCON:
		if b == C_ZCON || b == C_BITCON || b == C_ADDCON || b == C_ADDCON0 || b == C_ABCON || b == C_MBCON || b == C_MOVCON {
			return true
		}

	case C_VCON:
		if b == C_VCONADDR {
			return true
		} else {

			return cmp(C_LCON, b)
		}
		fallthrough

	case C_LACON:
		if b == C_AACON {
			return true
		}

	case C_SEXT2:
		if b == C_SEXT1 {
			return true
		}

	case C_SEXT4:
		if b == C_SEXT1 || b == C_SEXT2 {
			return true
		}

	case C_SEXT8:
		if b >= C_SEXT1 && b <= C_SEXT4 {
			return true
		}

	case C_SEXT16:
		if b >= C_SEXT1 && b <= C_SEXT8 {
			return true
		}

	case C_LEXT:
		if b >= C_SEXT1 && b <= C_SEXT16 {
			return true
		}

	case C_PPAUTO:
		if b == C_PSAUTO {
			return true
		}

	case C_UAUTO4K:
		if b == C_PSAUTO || b == C_PPAUTO {
			return true
		}

	case C_UAUTO8K:
		return cmp(C_UAUTO4K, b)

	case C_UAUTO16K:
		return cmp(C_UAUTO8K, b)

	case C_UAUTO32K:
		return cmp(C_UAUTO16K, b)

	case C_UAUTO64K:
		return cmp(C_UAUTO32K, b)

	case C_NPAUTO:
		return cmp(C_NSAUTO, b)

	case C_LAUTO:
		return cmp(C_NPAUTO, b) || cmp(C_UAUTO64K, b)

	case C_PSOREG:
		if b == C_ZOREG {
			return true
		}

	case C_PPOREG:
		if b == C_ZOREG || b == C_PSOREG {
			return true
		}

	case C_UOREG4K:
		if b == C_ZOREG || b == C_PSAUTO || b == C_PSOREG || b == C_PPAUTO || b == C_PPOREG {
			return true
		}

	case C_UOREG8K:
		return cmp(C_UOREG4K, b)

	case C_UOREG16K:
		return cmp(C_UOREG8K, b)

	case C_UOREG32K:
		return cmp(C_UOREG16K, b)

	case C_UOREG64K:
		return cmp(C_UOREG32K, b)

	case C_NPOREG:
		return cmp(C_NSOREG, b)

	case C_LOREG:
		return cmp(C_NPOREG, b) || cmp(C_UOREG64K, b)

	case C_LBRA:
		if b == C_SBRA {
			return true
		}
		break
	}

	return false
}

type ocmp []Optab

func (x ocmp) Len() int {
	return len(x)
}

func (x ocmp) Swap(i, j int) {
	x[i], x[j] = x[j], x[i]
}

func (x ocmp) Less(i, j int) bool {
	var p1 *Optab
	var p2 *Optab
	var n int
	p1 = &x[i]
	p2 = &x[j]
	n = int(p1.as) - int(p2.as)
	if n != 0 {
		return n < 0
	}
	n = int(p1.a1) - int(p2.a1)
	if n != 0 {
		return n < 0
	}
	n = int(p1.a2) - int(p2.a2)
	if n != 0 {
		return n < 0
	}
	n = int(p1.a3) - int(p2.a3)
	if n != 0 {
		return n < 0
	}
	return false
}

func buildop(ctxt *obj.Link) {
	var i int
	var n int
	var r int
	var t Oprang
	for i = 0; i < C_GOK; i++ {
		for n = 0; n < C_GOK; n++ {
			if cmp(n, i) {
				xcmp[i][n] = 1
			}
		}
	}
	for n = 0; optab[n].as != obj.AXXX; n++ {

	}
	sort.Sort(ocmp(optab[:n]))
	for i = 0; i < n; i++ {
		r = int(optab[i].as)
		oprange[r].start = optab[i:]
		for int(optab[i].as) == r {
			i++
		}
		oprange[r].stop = optab[i:]
		i--
		t = oprange[r]
		switch r {
		default:
			ctxt.Diag("unknown op in build: %v", Aconv(r))
			log.Fatalf("bad code")

		case AADD:
			oprange[AADDS] = t
			oprange[ASUB] = t
			oprange[ASUBS] = t
			oprange[AADDW] = t
			oprange[AADDSW] = t
			oprange[ASUBW] = t
			oprange[ASUBSW] = t

		case AAND: /* logical immediate, logical shifted register */
			oprange[AANDS] = t

			oprange[AANDSW] = t
			oprange[AANDW] = t
			oprange[AEOR] = t
			oprange[AEORW] = t
			oprange[AORR] = t
			oprange[AORRW] = t

		case ABIC: /* only logical shifted register */
			oprange[ABICS] = t

			oprange[ABICSW] = t
			oprange[ABICW] = t
			oprange[AEON] = t
			oprange[AEONW] = t
			oprange[AORN] = t
			oprange[AORNW] = t

		case ANEG:
			oprange[ANEGS] = t
			oprange[ANEGSW] = t
			oprange[ANEGW] = t

		case AADC: /* rn=Rd */
			oprange[AADCW] = t

			oprange[AADCS] = t
			oprange[AADCSW] = t
			oprange[ASBC] = t
			oprange[ASBCW] = t
			oprange[ASBCS] = t
			oprange[ASBCSW] = t

		case ANGC: /* rn=REGZERO */
			oprange[ANGCW] = t

			oprange[ANGCS] = t
			oprange[ANGCSW] = t

		case ACMP:
			oprange[ACMPW] = t
			oprange[ACMN] = t
			oprange[ACMNW] = t

		case ATST:
			oprange[ATSTW] = t

			/* register/register, and shifted */
		case AMVN:
			oprange[AMVNW] = t

		case AMOVK:
			oprange[AMOVKW] = t
			oprange[AMOVN] = t
			oprange[AMOVNW] = t
			oprange[AMOVZ] = t
			oprange[AMOVZW] = t

		case ABEQ:
			oprange[ABNE] = t
			oprange[ABCS] = t
			oprange[ABHS] = t
			oprange[ABCC] = t
			oprange[ABLO] = t
			oprange[ABMI] = t
			oprange[ABPL] = t
			oprange[ABVS] = t
			oprange[ABVC] = t
			oprange[ABHI] = t
			oprange[ABLS] = t
			oprange[ABGE] = t
			oprange[ABLT] = t
			oprange[ABGT] = t
			oprange[ABLE] = t

		case ALSL:
			oprange[ALSLW] = t
			oprange[ALSR] = t
			oprange[ALSRW] = t
			oprange[AASR] = t
			oprange[AASRW] = t
			oprange[AROR] = t
			oprange[ARORW] = t

		case ACLS:
			oprange[ACLSW] = t
			oprange[ACLZ] = t
			oprange[ACLZW] = t
			oprange[ARBIT] = t
			oprange[ARBITW] = t
			oprange[AREV] = t
			oprange[AREVW] = t
			oprange[AREV16] = t
			oprange[AREV16W] = t
			oprange[AREV32] = t

		case ASDIV:
			oprange[ASDIVW] = t
			oprange[AUDIV] = t
			oprange[AUDIVW] = t
			oprange[ACRC32B] = t
			oprange[ACRC32CB] = t
			oprange[ACRC32CH] = t
			oprange[ACRC32CW] = t
			oprange[ACRC32CX] = t
			oprange[ACRC32H] = t
			oprange[ACRC32W] = t
			oprange[ACRC32X] = t

		case AMADD:
			oprange[AMADDW] = t
			oprange[AMSUB] = t
			oprange[AMSUBW] = t
			oprange[ASMADDL] = t
			oprange[ASMSUBL] = t
			oprange[AUMADDL] = t
			oprange[AUMSUBL] = t

		case AREM:
			oprange[AREMW] = t
			oprange[AUREM] = t
			oprange[AUREMW] = t

		case AMUL:
			oprange[AMULW] = t
			oprange[AMNEG] = t
			oprange[AMNEGW] = t
			oprange[ASMNEGL] = t
			oprange[ASMULL] = t
			oprange[ASMULH] = t
			oprange[AUMNEGL] = t
			oprange[AUMULH] = t
			oprange[AUMULL] = t

		case AMOVB:
			oprange[AMOVBU] = t

		case AMOVH:
			oprange[AMOVHU] = t

		case AMOVW:
			oprange[AMOVWU] = t

		case ABFM:
			oprange[ABFMW] = t
			oprange[ASBFM] = t
			oprange[ASBFMW] = t
			oprange[AUBFM] = t
			oprange[AUBFMW] = t

		case ABFI:
			oprange[ABFIW] = t
			oprange[ABFXIL] = t
			oprange[ABFXILW] = t
			oprange[ASBFIZ] = t
			oprange[ASBFIZW] = t
			oprange[ASBFX] = t
			oprange[ASBFXW] = t
			oprange[AUBFIZ] = t
			oprange[AUBFIZW] = t
			oprange[AUBFX] = t
			oprange[AUBFXW] = t

		case AEXTR:
			oprange[AEXTRW] = t

		case ASXTB:
			oprange[ASXTBW] = t
			oprange[ASXTH] = t
			oprange[ASXTHW] = t
			oprange[ASXTW] = t
			oprange[AUXTB] = t
			oprange[AUXTH] = t
			oprange[AUXTW] = t
			oprange[AUXTBW] = t
			oprange[AUXTHW] = t

		case ACCMN:
			oprange[ACCMNW] = t
			oprange[ACCMP] = t
			oprange[ACCMPW] = t

		case ACSEL:
			oprange[ACSELW] = t
			oprange[ACSINC] = t
			oprange[ACSINCW] = t
			oprange[ACSINV] = t
			oprange[ACSINVW] = t
			oprange[ACSNEG] = t
			oprange[ACSNEGW] = t

			// aliases Rm=Rn, !cond
			oprange[ACINC] = t

			oprange[ACINCW] = t
			oprange[ACINV] = t
			oprange[ACINVW] = t
			oprange[ACNEG] = t
			oprange[ACNEGW] = t

			// aliases, Rm=Rn=REGZERO, !cond
		case ACSET:
			oprange[ACSETW] = t

			oprange[ACSETM] = t
			oprange[ACSETMW] = t

		case AMOV,
			AMOVBU,
			AB,
			ABL,
			AWORD,
			ADWORD,
			ARET,
			ATEXT,
			ACASE,
			ABCASE,
			ASTP,
			ALDP:
			break

		case AERET:
			oprange[ANOP] = t
			oprange[AWFE] = t
			oprange[AWFI] = t
			oprange[AYIELD] = t
			oprange[ASEV] = t
			oprange[ASEVL] = t
			oprange[ADRPS] = t

		case ACBZ:
			oprange[ACBZW] = t
			oprange[ACBNZ] = t
			oprange[ACBNZW] = t

		case ATBZ:
			oprange[ATBNZ] = t

		case AADR,
			AADRP:
			break

		case ACLREX:
			break

		case ASVC:
			oprange[AHLT] = t
			oprange[AHVC] = t
			oprange[ASMC] = t
			oprange[ABRK] = t
			oprange[ADCPS1] = t
			oprange[ADCPS2] = t
			oprange[ADCPS3] = t

		case AFADDS:
			oprange[AFADDD] = t
			oprange[AFSUBS] = t
			oprange[AFSUBD] = t
			oprange[AFMULS] = t
			oprange[AFMULD] = t
			oprange[AFNMULS] = t
			oprange[AFNMULD] = t
			oprange[AFDIVS] = t
			oprange[AFMAXD] = t
			oprange[AFMAXS] = t
			oprange[AFMIND] = t
			oprange[AFMINS] = t
			oprange[AFMAXNMD] = t
			oprange[AFMAXNMS] = t
			oprange[AFMINNMD] = t
			oprange[AFMINNMS] = t
			oprange[AFDIVD] = t

		case AFCVTSD:
			oprange[AFCVTDS] = t
			oprange[AFABSD] = t
			oprange[AFABSS] = t
			oprange[AFNEGD] = t
			oprange[AFNEGS] = t
			oprange[AFSQRTD] = t
			oprange[AFSQRTS] = t
			oprange[AFRINTNS] = t
			oprange[AFRINTND] = t
			oprange[AFRINTPS] = t
			oprange[AFRINTPD] = t
			oprange[AFRINTMS] = t
			oprange[AFRINTMD] = t
			oprange[AFRINTZS] = t
			oprange[AFRINTZD] = t
			oprange[AFRINTAS] = t
			oprange[AFRINTAD] = t
			oprange[AFRINTXS] = t
			oprange[AFRINTXD] = t
			oprange[AFRINTIS] = t
			oprange[AFRINTID] = t
			oprange[AFCVTDH] = t
			oprange[AFCVTHS] = t
			oprange[AFCVTHD] = t
			oprange[AFCVTSH] = t

		case AFCMPS:
			oprange[AFCMPD] = t
			oprange[AFCMPES] = t
			oprange[AFCMPED] = t

		case AFCCMPS:
			oprange[AFCCMPD] = t
			oprange[AFCCMPES] = t
			oprange[AFCCMPED] = t

		case AFCSELD:
			oprange[AFCSELS] = t

		case AFMOVS,
			AFMOVD:
			break

		case AFCVTZSD:
			oprange[AFCVTZSDW] = t
			oprange[AFCVTZSS] = t
			oprange[AFCVTZSSW] = t
			oprange[AFCVTZUD] = t
			oprange[AFCVTZUDW] = t
			oprange[AFCVTZUS] = t
			oprange[AFCVTZUSW] = t

		case ASCVTFD:
			oprange[ASCVTFS] = t
			oprange[ASCVTFWD] = t
			oprange[ASCVTFWS] = t
			oprange[AUCVTFD] = t
			oprange[AUCVTFS] = t
			oprange[AUCVTFWD] = t
			oprange[AUCVTFWS] = t

		case ASYS:
			oprange[AAT] = t
			oprange[ADC] = t
			oprange[AIC] = t
			oprange[ATLBI] = t

		case ASYSL,
			AHINT:
			break

		case ADMB:
			oprange[ADSB] = t
			oprange[AISB] = t

		case AMRS,
			AMSR:
			break

		case ALDAR:
			oprange[ALDARW] = t
			fallthrough

		case ALDXR:
			oprange[ALDXRB] = t
			oprange[ALDXRH] = t
			oprange[ALDXRW] = t

		case ALDAXR:
			oprange[ALDAXRW] = t

		case ALDXP:
			oprange[ALDXPW] = t

		case ASTLR:
			oprange[ASTLRW] = t

		case ASTXR:
			oprange[ASTXRB] = t
			oprange[ASTXRH] = t
			oprange[ASTXRW] = t

		case ASTLXR:
			oprange[ASTLXRW] = t

		case ASTXP:
			oprange[ASTXPW] = t

		case AAESD:
			oprange[AAESE] = t
			oprange[AAESMC] = t
			oprange[AAESIMC] = t
			oprange[ASHA1H] = t
			oprange[ASHA1SU1] = t
			oprange[ASHA256SU0] = t

		case ASHA1C:
			oprange[ASHA1P] = t
			oprange[ASHA1M] = t
			oprange[ASHA1SU0] = t
			oprange[ASHA256H] = t
			oprange[ASHA256H2] = t
			oprange[ASHA256SU1] = t

		case AUNDEF,
			AUSEFIELD,
			AFUNCDATA,
			APCDATA,
			ADUFFZERO,
			ADUFFCOPY:
			break
		}
	}
}

func chipfloat7(ctxt *obj.Link, e float64) int {
	var n int
	var h1 uint32
	var l uint32
	var h uint32
	var ei uint64

	ei = math.Float64bits(e)
	l = uint32(int32(ei))
	h = uint32(int32(ei >> 32))

	if l != 0 || h&0xffff != 0 {
		goto no
	}
	h1 = h & 0x7fc00000
	if h1 != 0x40000000 && h1 != 0x3fc00000 {
		goto no
	}
	n = 0

	// sign bit (a)
	if h&0x80000000 != 0 {

		n |= 1 << 7
	}

	// exp sign bit (b)
	if h1 == 0x3fc00000 {

		n |= 1 << 6
	}

	// rest of exp and mantissa (cd-efgh)
	n |= int((h >> 16) & 0x3f)

	//print("match %.8lux %.8lux %d\n", l, h, n);
	return n

no:
	return -1
}

func asmout(ctxt *obj.Link, p *obj.Prog, o *Optab, out []uint32) {
}

/*
 * basic Rm op Rn -> Rd (using shifted register with 0)
 * also op Rn -> Rt
 * also Rm*Rn op Ra -> Rd
 */
func oprrr(ctxt *obj.Link, a int) uint32 {

	switch a {
	case AADC:
		return S64 | 0<<30 | 0<<29 | 0xd0<<21 | 0<<10

	case AADCW:
		return S32 | 0<<30 | 0<<29 | 0xd0<<21 | 0<<10

	case AADCS:
		return S64 | 0<<30 | 1<<29 | 0xd0<<21 | 0<<10

	case AADCSW:
		return S32 | 0<<30 | 1<<29 | 0xd0<<21 | 0<<10

	case ANGC,
		ASBC:
		return S64 | 1<<30 | 0<<29 | 0xd0<<21 | 0<<10

	case ANGCS,
		ASBCS:
		return S64 | 1<<30 | 1<<29 | 0xd0<<21 | 0<<10

	case ANGCW,
		ASBCW:
		return S32 | 1<<30 | 0<<29 | 0xd0<<21 | 0<<10

	case ANGCSW,
		ASBCSW:
		return S32 | 1<<30 | 1<<29 | 0xd0<<21 | 0<<10

	case AADD:
		return S64 | 0<<30 | 0<<29 | 0x0b<<24 | 0<<22 | 0<<21 | 0<<10

	case AADDW:
		return S32 | 0<<30 | 0<<29 | 0x0b<<24 | 0<<22 | 0<<21 | 0<<10

	case ACMN,
		AADDS:
		return S64 | 0<<30 | 1<<29 | 0x0b<<24 | 0<<22 | 0<<21 | 0<<10

	case ACMNW,
		AADDSW:
		return S32 | 0<<30 | 1<<29 | 0x0b<<24 | 0<<22 | 0<<21 | 0<<10

	case ASUB:
		return S64 | 1<<30 | 0<<29 | 0x0b<<24 | 0<<22 | 0<<21 | 0<<10

	case ASUBW:
		return S32 | 1<<30 | 0<<29 | 0x0b<<24 | 0<<22 | 0<<21 | 0<<10

	case ACMP,
		ASUBS:
		return S64 | 1<<30 | 1<<29 | 0x0b<<24 | 0<<22 | 0<<21 | 0<<10

	case ACMPW,
		ASUBSW:
		return S32 | 1<<30 | 1<<29 | 0x0b<<24 | 0<<22 | 0<<21 | 0<<10

	case AAND:
		return S64 | 0<<29 | 0xA<<24

	case AANDW:
		return S32 | 0<<29 | 0xA<<24

	case AMOV,
		AORR:
		return S64 | 1<<29 | 0xA<<24

		//	case AMOVW:
	case AMOVWU,
		AORRW:
		return S32 | 1<<29 | 0xA<<24

	case AEOR:
		return S64 | 2<<29 | 0xA<<24

	case AEORW:
		return S32 | 2<<29 | 0xA<<24

	case AANDS:
		return S64 | 3<<29 | 0xA<<24

	case AANDSW:
		return S32 | 3<<29 | 0xA<<24

	case ABIC:
		return S64 | 0<<29 | 0xA<<24 | 1<<21

	case ABICW:
		return S32 | 0<<29 | 0xA<<24 | 1<<21

	case ABICS:
		return S64 | 3<<29 | 0xA<<24 | 1<<21

	case ABICSW:
		return S32 | 3<<29 | 0xA<<24 | 1<<21

	case AEON:
		return S64 | 2<<29 | 0xA<<24 | 1<<21

	case AEONW:
		return S32 | 2<<29 | 0xA<<24 | 1<<21

	case AMVN,
		AORN:
		return S64 | 1<<29 | 0xA<<24 | 1<<21

	case AMVNW,
		AORNW:
		return S32 | 1<<29 | 0xA<<24 | 1<<21

	case AASR:
		return S64 | OPDP2(10) /* also ASRV */

	case AASRW:
		return S32 | OPDP2(10)

	case ALSL:
		return S64 | OPDP2(8)

	case ALSLW:
		return S32 | OPDP2(8)

	case ALSR:
		return S64 | OPDP2(9)

	case ALSRW:
		return S32 | OPDP2(9)

	case AROR:
		return S64 | OPDP2(11)

	case ARORW:
		return S32 | OPDP2(11)

	case ACCMN:
		return S64 | 0<<30 | 1<<29 | 0xD2<<21 | 0<<11 | 0<<10 | 0<<4 /* cond<<12 | nzcv<<0 */

	case ACCMNW:
		return S32 | 0<<30 | 1<<29 | 0xD2<<21 | 0<<11 | 0<<10 | 0<<4

	case ACCMP:
		return S64 | 1<<30 | 1<<29 | 0xD2<<21 | 0<<11 | 0<<10 | 0<<4 /* imm5<<16 | cond<<12 | nzcv<<0 */

	case ACCMPW:
		return S32 | 1<<30 | 1<<29 | 0xD2<<21 | 0<<11 | 0<<10 | 0<<4

	case ACRC32B:
		return S32 | OPDP2(16)

	case ACRC32H:
		return S32 | OPDP2(17)

	case ACRC32W:
		return S32 | OPDP2(18)

	case ACRC32X:
		return S64 | OPDP2(19)

	case ACRC32CB:
		return S32 | OPDP2(20)

	case ACRC32CH:
		return S32 | OPDP2(21)

	case ACRC32CW:
		return S32 | OPDP2(22)

	case ACRC32CX:
		return S64 | OPDP2(23)

	case ACSEL:
		return S64 | 0<<30 | 0<<29 | 0xD4<<21 | 0<<11 | 0<<10

	case ACSELW:
		return S32 | 0<<30 | 0<<29 | 0xD4<<21 | 0<<11 | 0<<10

	case ACSET:
		return S64 | 0<<30 | 0<<29 | 0xD4<<21 | 0<<11 | 1<<10

	case ACSETW:
		return S32 | 0<<30 | 0<<29 | 0xD4<<21 | 0<<11 | 1<<10

	case ACSETM:
		return S64 | 1<<30 | 0<<29 | 0xD4<<21 | 0<<11 | 0<<10

	case ACSETMW:
		return S32 | 1<<30 | 0<<29 | 0xD4<<21 | 0<<11 | 0<<10

	case ACINC,
		ACSINC:
		return S64 | 0<<30 | 0<<29 | 0xD4<<21 | 0<<11 | 1<<10

	case ACINCW,
		ACSINCW:
		return S32 | 0<<30 | 0<<29 | 0xD4<<21 | 0<<11 | 1<<10

	case ACINV,
		ACSINV:
		return S64 | 1<<30 | 0<<29 | 0xD4<<21 | 0<<11 | 0<<10

	case ACINVW,
		ACSINVW:
		return S32 | 1<<30 | 0<<29 | 0xD4<<21 | 0<<11 | 0<<10

	case ACNEG,
		ACSNEG:
		return S64 | 1<<30 | 0<<29 | 0xD4<<21 | 0<<11 | 1<<10

	case ACNEGW,
		ACSNEGW:
		return S32 | 1<<30 | 0<<29 | 0xD4<<21 | 0<<11 | 1<<10

	case AMUL,
		AMADD:
		return S64 | 0<<29 | 0x1B<<24 | 0<<21 | 0<<15

	case AMULW,
		AMADDW:
		return S32 | 0<<29 | 0x1B<<24 | 0<<21 | 0<<15

	case AMNEG,
		AMSUB:
		return S64 | 0<<29 | 0x1B<<24 | 0<<21 | 1<<15

	case AMNEGW,
		AMSUBW:
		return S32 | 0<<29 | 0x1B<<24 | 0<<21 | 1<<15

	case AMRS:
		return SYSOP(1, 2, 0, 0, 0, 0, 0)

	case AMSR:
		return SYSOP(0, 2, 0, 0, 0, 0, 0)

	case ANEG:
		return S64 | 1<<30 | 0<<29 | 0xB<<24 | 0<<21

	case ANEGW:
		return S32 | 1<<30 | 0<<29 | 0xB<<24 | 0<<21

	case ANEGS:
		return S64 | 1<<30 | 1<<29 | 0xB<<24 | 0<<21

	case ANEGSW:
		return S32 | 1<<30 | 1<<29 | 0xB<<24 | 0<<21

	case AREM,
		ASDIV:
		return S64 | OPDP2(3)

	case AREMW,
		ASDIVW:
		return S32 | OPDP2(3)

	case ASMULL,
		ASMADDL:
		return OPDP3(1, 0, 1, 0)

	case ASMNEGL,
		ASMSUBL:
		return OPDP3(1, 0, 1, 1)

	case ASMULH:
		return OPDP3(1, 0, 2, 0)

	case AUMULL,
		AUMADDL:
		return OPDP3(1, 0, 5, 0)

	case AUMNEGL,
		AUMSUBL:
		return OPDP3(1, 0, 5, 1)

	case AUMULH:
		return OPDP3(1, 0, 6, 0)

	case AUREM,
		AUDIV:
		return S64 | OPDP2(2)

	case AUREMW,
		AUDIVW:
		return S32 | OPDP2(2)

	case AAESE:
		return 0x4E<<24 | 2<<20 | 8<<16 | 4<<12 | 2<<10

	case AAESD:
		return 0x4E<<24 | 2<<20 | 8<<16 | 5<<12 | 2<<10

	case AAESMC:
		return 0x4E<<24 | 2<<20 | 8<<16 | 6<<12 | 2<<10

	case AAESIMC:
		return 0x4E<<24 | 2<<20 | 8<<16 | 7<<12 | 2<<10

	case ASHA1C:
		return 0x5E<<24 | 0<<12

	case ASHA1P:
		return 0x5E<<24 | 1<<12

	case ASHA1M:
		return 0x5E<<24 | 2<<12

	case ASHA1SU0:
		return 0x5E<<24 | 3<<12

	case ASHA256H:
		return 0x5E<<24 | 4<<12

	case ASHA256H2:
		return 0x5E<<24 | 5<<12

	case ASHA256SU1:
		return 0x5E<<24 | 6<<12

	case ASHA1H:
		return 0x5E<<24 | 2<<20 | 8<<16 | 0<<12 | 2<<10

	case ASHA1SU1:
		return 0x5E<<24 | 2<<20 | 8<<16 | 1<<12 | 2<<10

	case ASHA256SU0:
		return 0x5E<<24 | 2<<20 | 8<<16 | 2<<12 | 2<<10

	case AFCVTZSD:
		return FPCVTI(1, 0, 1, 3, 0)

	case AFCVTZSDW:
		return FPCVTI(0, 0, 1, 3, 0)

	case AFCVTZSS:
		return FPCVTI(1, 0, 0, 3, 0)

	case AFCVTZSSW:
		return FPCVTI(0, 0, 0, 3, 0)

	case AFCVTZUD:
		return FPCVTI(1, 0, 1, 3, 1)

	case AFCVTZUDW:
		return FPCVTI(0, 0, 1, 3, 1)

	case AFCVTZUS:
		return FPCVTI(1, 0, 0, 3, 1)

	case AFCVTZUSW:
		return FPCVTI(0, 0, 0, 3, 1)

	case ASCVTFD:
		return FPCVTI(1, 0, 1, 0, 2)

	case ASCVTFS:
		return FPCVTI(1, 0, 0, 0, 2)

	case ASCVTFWD:
		return FPCVTI(0, 0, 1, 0, 2)

	case ASCVTFWS:
		return FPCVTI(0, 0, 0, 0, 2)

	case AUCVTFD:
		return FPCVTI(1, 0, 1, 0, 3)

	case AUCVTFS:
		return FPCVTI(1, 0, 0, 0, 3)

	case AUCVTFWD:
		return FPCVTI(0, 0, 1, 0, 3)

	case AUCVTFWS:
		return FPCVTI(0, 0, 0, 0, 3)

	case AFADDS:
		return FPOP2S(0, 0, 0, 2)

	case AFADDD:
		return FPOP2S(0, 0, 1, 2)

	case AFSUBS:
		return FPOP2S(0, 0, 0, 3)

	case AFSUBD:
		return FPOP2S(0, 0, 1, 3)

	case AFMULS:
		return FPOP2S(0, 0, 0, 0)

	case AFMULD:
		return FPOP2S(0, 0, 1, 0)

	case AFDIVS:
		return FPOP2S(0, 0, 0, 1)

	case AFDIVD:
		return FPOP2S(0, 0, 1, 1)

	case AFMAXS:
		return FPOP2S(0, 0, 0, 4)

	case AFMINS:
		return FPOP2S(0, 0, 0, 5)

	case AFMAXD:
		return FPOP2S(0, 0, 1, 4)

	case AFMIND:
		return FPOP2S(0, 0, 1, 5)

	case AFMAXNMS:
		return FPOP2S(0, 0, 0, 6)

	case AFMAXNMD:
		return FPOP2S(0, 0, 1, 6)

	case AFMINNMS:
		return FPOP2S(0, 0, 0, 7)

	case AFMINNMD:
		return FPOP2S(0, 0, 1, 7)

	case AFNMULS:
		return FPOP2S(0, 0, 0, 8)

	case AFNMULD:
		return FPOP2S(0, 0, 1, 8)

	case AFCMPS:
		return FPCMP(0, 0, 0, 0, 0)

	case AFCMPD:
		return FPCMP(0, 0, 1, 0, 0)

	case AFCMPES:
		return FPCMP(0, 0, 0, 0, 16)

	case AFCMPED:
		return FPCMP(0, 0, 1, 0, 16)

	case AFCCMPS:
		return FPCCMP(0, 0, 0, 0)

	case AFCCMPD:
		return FPCCMP(0, 0, 1, 0)

	case AFCCMPES:
		return FPCCMP(0, 0, 0, 1)

	case AFCCMPED:
		return FPCCMP(0, 0, 1, 1)

	case AFCSELS:
		return 0x1E<<24 | 0<<22 | 1<<21 | 3<<10

	case AFCSELD:
		return 0x1E<<24 | 1<<22 | 1<<21 | 3<<10

	case AFMOVS:
		return FPOP1S(0, 0, 0, 0)

	case AFABSS:
		return FPOP1S(0, 0, 0, 1)

	case AFNEGS:
		return FPOP1S(0, 0, 0, 2)

	case AFSQRTS:
		return FPOP1S(0, 0, 0, 3)

	case AFCVTSD:
		return FPOP1S(0, 0, 0, 5)

	case AFCVTSH:
		return FPOP1S(0, 0, 0, 7)

	case AFRINTNS:
		return FPOP1S(0, 0, 0, 8)

	case AFRINTPS:
		return FPOP1S(0, 0, 0, 9)

	case AFRINTMS:
		return FPOP1S(0, 0, 0, 10)

	case AFRINTZS:
		return FPOP1S(0, 0, 0, 11)

	case AFRINTAS:
		return FPOP1S(0, 0, 0, 12)

	case AFRINTXS:
		return FPOP1S(0, 0, 0, 14)

	case AFRINTIS:
		return FPOP1S(0, 0, 0, 15)

	case AFMOVD:
		return FPOP1S(0, 0, 1, 0)

	case AFABSD:
		return FPOP1S(0, 0, 1, 1)

	case AFNEGD:
		return FPOP1S(0, 0, 1, 2)

	case AFSQRTD:
		return FPOP1S(0, 0, 1, 3)

	case AFCVTDS:
		return FPOP1S(0, 0, 1, 4)

	case AFCVTDH:
		return FPOP1S(0, 0, 1, 7)

	case AFRINTND:
		return FPOP1S(0, 0, 1, 8)

	case AFRINTPD:
		return FPOP1S(0, 0, 1, 9)

	case AFRINTMD:
		return FPOP1S(0, 0, 1, 10)

	case AFRINTZD:
		return FPOP1S(0, 0, 1, 11)

	case AFRINTAD:
		return FPOP1S(0, 0, 1, 12)

	case AFRINTXD:
		return FPOP1S(0, 0, 1, 14)

	case AFRINTID:
		return FPOP1S(0, 0, 1, 15)

	case AFCVTHS:
		return FPOP1S(0, 0, 3, 4)

	case AFCVTHD:
		return FPOP1S(0, 0, 3, 5)
	}

	ctxt.Diag("bad rrr %d %v", a, Aconv(a))
	prasm(ctxt.Curp)
	return 0
}

/*
 * imm -> Rd
 * imm op Rn -> Rd
 */
func opirr(ctxt *obj.Link, a int) uint32 {

	switch a {
	/* op $addcon, Rn, Rd */
	case AMOV,
		AADD:
		return S64 | 0<<30 | 0<<29 | 0x11<<24

	case ACMN,
		AADDS:
		return S64 | 0<<30 | 1<<29 | 0x11<<24

	case AMOVW,
		AADDW:
		return S32 | 0<<30 | 0<<29 | 0x11<<24

	case ACMNW,
		AADDSW:
		return S32 | 0<<30 | 1<<29 | 0x11<<24

	case ASUB:
		return S64 | 1<<30 | 0<<29 | 0x11<<24

	case ACMP,
		ASUBS:
		return S64 | 1<<30 | 1<<29 | 0x11<<24

	case ASUBW:
		return S32 | 1<<30 | 0<<29 | 0x11<<24

	case ACMPW,
		ASUBSW:
		return S32 | 1<<30 | 1<<29 | 0x11<<24

		/* op $imm(SB), Rd; op label, Rd */
	case AADR:
		return 0<<31 | 0x10<<24

	case AADRP:
		return 1<<31 | 0x10<<24

		/* op $bimm, Rn, Rd */
	case AAND:
		return S64 | 0<<29 | 0x24<<23

	case AANDW:
		return S32 | 0<<29 | 0x24<<23 | 0<<22

	case AORR:
		return S64 | 1<<29 | 0x24<<23

	case AORRW:
		return S32 | 1<<29 | 0x24<<23 | 0<<22

	case AEOR:
		return S64 | 2<<29 | 0x24<<23

	case AEORW:
		return S32 | 2<<29 | 0x24<<23 | 0<<22

	case AANDS:
		return S64 | 3<<29 | 0x24<<23

	case AANDSW:
		return S32 | 3<<29 | 0x24<<23 | 0<<22

	case AASR:
		return S64 | 0<<29 | 0x26<<23 /* alias of SBFM */

	case AASRW:
		return S32 | 0<<29 | 0x26<<23 | 0<<22

		/* op $width, $lsb, Rn, Rd */
	case ABFI:
		return S64 | 2<<29 | 0x26<<23 | 1<<22
		/* alias of BFM */

	case ABFIW:
		return S32 | 2<<29 | 0x26<<23 | 0<<22

		/* op $imms, $immr, Rn, Rd */
	case ABFM:
		return S64 | 1<<29 | 0x26<<23 | 1<<22

	case ABFMW:
		return S32 | 1<<29 | 0x26<<23 | 0<<22

	case ASBFM:
		return S64 | 0<<29 | 0x26<<23 | 1<<22

	case ASBFMW:
		return S32 | 0<<29 | 0x26<<23 | 0<<22

	case AUBFM:
		return S64 | 2<<29 | 0x26<<23 | 1<<22

	case AUBFMW:
		return S32 | 2<<29 | 0x26<<23 | 0<<22

	case ABFXIL:
		return S64 | 1<<29 | 0x26<<23 | 1<<22 /* alias of BFM */

	case ABFXILW:
		return S32 | 1<<29 | 0x26<<23 | 0<<22

	case AEXTR:
		return S64 | 0<<29 | 0x27<<23 | 1<<22 | 0<<21

	case AEXTRW:
		return S32 | 0<<29 | 0x27<<23 | 0<<22 | 0<<21

	case ACBNZ:
		return S64 | 0x1A<<25 | 1<<24

	case ACBNZW:
		return S32 | 0x1A<<25 | 1<<24

	case ACBZ:
		return S64 | 0x1A<<25 | 0<<24

	case ACBZW:
		return S32 | 0x1A<<25 | 0<<24

	case ACCMN:
		return S64 | 0<<30 | 1<<29 | 0xD2<<21 | 1<<11 | 0<<10 | 0<<4 /* imm5<<16 | cond<<12 | nzcv<<0 */

	case ACCMNW:
		return S32 | 0<<30 | 1<<29 | 0xD2<<21 | 1<<11 | 0<<10 | 0<<4

	case ACCMP:
		return S64 | 1<<30 | 1<<29 | 0xD2<<21 | 1<<11 | 0<<10 | 0<<4 /* imm5<<16 | cond<<12 | nzcv<<0 */

	case ACCMPW:
		return S32 | 1<<30 | 1<<29 | 0xD2<<21 | 1<<11 | 0<<10 | 0<<4

	case AMOVK:
		return S64 | 3<<29 | 0x25<<23

	case AMOVKW:
		return S32 | 3<<29 | 0x25<<23

	case AMOVN:
		return S64 | 0<<29 | 0x25<<23

	case AMOVNW:
		return S32 | 0<<29 | 0x25<<23

	case AMOVZ:
		return S64 | 2<<29 | 0x25<<23

	case AMOVZW:
		return S32 | 2<<29 | 0x25<<23

	case AMSR:
		return SYSOP(0, 0, 0, 4, 0, 0, 0x1F) /* MSR (immediate) */

	case AAT,
		ADC,
		AIC,
		ATLBI,
		ASYS:
		return SYSOP(0, 1, 0, 0, 0, 0, 0)

	case ASYSL:
		return SYSOP(1, 1, 0, 0, 0, 0, 0)

	case ATBZ:
		return 0x36 << 24

	case ATBNZ:
		return 0x37 << 24

	case ADSB:
		return SYSOP(0, 0, 3, 3, 0, 4, 0x1F)

	case ADMB:
		return SYSOP(0, 0, 3, 3, 0, 5, 0x1F)

	case AISB:
		return SYSOP(0, 0, 3, 3, 0, 6, 0x1F)

	case AHINT:
		return SYSOP(0, 0, 3, 2, 0, 0, 0x1F)
	}

	ctxt.Diag("bad irr %v", Aconv(a))
	prasm(ctxt.Curp)
	return 0
}

func opbit(ctxt *obj.Link, a int) uint32 {
	switch a {
	case ACLS:
		return S64 | OPBIT(5)

	case ACLSW:
		return S32 | OPBIT(5)

	case ACLZ:
		return S64 | OPBIT(4)

	case ACLZW:
		return S32 | OPBIT(4)

	case ARBIT:
		return S64 | OPBIT(0)

	case ARBITW:
		return S32 | OPBIT(0)

	case AREV:
		return S64 | OPBIT(3)

	case AREVW:
		return S32 | OPBIT(2)

	case AREV16:
		return S64 | OPBIT(1)

	case AREV16W:
		return S32 | OPBIT(1)

	case AREV32:
		return S64 | OPBIT(2)

	default:
		ctxt.Diag("bad bit op\n%v", ctxt.Curp)
		return 0
	}
}

/*
 * add/subtract extended register
 */
func opxrrr(ctxt *obj.Link, a int) uint32 {

	switch a {
	case AADD:
		return S64 | 0<<30 | 0<<29 | 0x0b<<24 | 0<<22 | 1<<21 | LSL0_64

	case AADDW:
		return S32 | 0<<30 | 0<<29 | 0x0b<<24 | 0<<22 | 1<<21 | LSL0_32

	case ACMN,
		AADDS:
		return S64 | 0<<30 | 1<<29 | 0x0b<<24 | 0<<22 | 1<<21 | LSL0_64

	case ACMNW,
		AADDSW:
		return S32 | 0<<30 | 1<<29 | 0x0b<<24 | 0<<22 | 1<<21 | LSL0_32

	case ASUB:
		return S64 | 1<<30 | 0<<29 | 0x0b<<24 | 0<<22 | 1<<21 | LSL0_64

	case ASUBW:
		return S32 | 1<<30 | 0<<29 | 0x0b<<24 | 0<<22 | 1<<21 | LSL0_32

	case ACMP,
		ASUBS:
		return S64 | 1<<30 | 1<<29 | 0x0b<<24 | 0<<22 | 1<<21 | LSL0_64

	case ACMPW,
		ASUBSW:
		return S32 | 1<<30 | 1<<29 | 0x0b<<24 | 0<<22 | 1<<21 | LSL0_32
	}

	ctxt.Diag("bad opxrrr %v\n%v", Aconv(a), ctxt.Curp)
	return 0
}

func opimm(ctxt *obj.Link, a int) uint32 {
	switch a {
	case ASVC:
		return 0xD4<<24 | 0<<21 | 1 /* imm16<<5 */

	case AHVC:
		return 0xD4<<24 | 0<<21 | 2

	case ASMC:
		return 0xD4<<24 | 0<<21 | 3

	case ABRK:
		return 0xD4<<24 | 1<<21 | 0

	case AHLT:
		return 0xD4<<24 | 2<<21 | 0

	case ADCPS1:
		return 0xD4<<24 | 5<<21 | 1

	case ADCPS2:
		return 0xD4<<24 | 5<<21 | 2

	case ADCPS3:
		return 0xD4<<24 | 5<<21 | 3

	case ACLREX:
		return SYSOP(0, 0, 3, 3, 0, 2, 0x1F)
	}

	ctxt.Diag("bad imm %v", Aconv(a))
	prasm(ctxt.Curp)
	return 0
}

func brdist(ctxt *obj.Link, p *obj.Prog, preshift int, flen int, shift int) int64 {
	var v int64
	var t int64
	v = 0
	t = 0
	if p.Pcond != nil {
		v = (p.Pcond.Pc >> uint(preshift)) - (ctxt.Pc >> uint(preshift))
		if (v & ((1 << uint(shift)) - 1)) != 0 {
			ctxt.Diag("misaligned label\n%v", p)
		}
		v >>= uint(shift)
		t = int64(1) << uint(flen-1)
		if v < -t || v >= t {
			ctxt.Diag("branch too far\n%v", p)
		}
	}

	return v & ((t << 1) - 1)
}

/*
 * pc-relative branches
 */
func opbra(ctxt *obj.Link, a int) uint32 {

	switch a {
	case ABEQ:
		return OPBcc(0x0)

	case ABNE:
		return OPBcc(0x1)

	case ABCS:
		return OPBcc(0x2)

	case ABHS:
		return OPBcc(0x2)

	case ABCC:
		return OPBcc(0x3)

	case ABLO:
		return OPBcc(0x3)

	case ABMI:
		return OPBcc(0x4)

	case ABPL:
		return OPBcc(0x5)

	case ABVS:
		return OPBcc(0x6)

	case ABVC:
		return OPBcc(0x7)

	case ABHI:
		return OPBcc(0x8)

	case ABLS:
		return OPBcc(0x9)

	case ABGE:
		return OPBcc(0xa)

	case ABLT:
		return OPBcc(0xb)

	case ABGT:
		return OPBcc(0xc)

	case ABLE:
		return OPBcc(0xd) /* imm19<<5 | cond */

	case AB:
		return 0<<31 | 5<<26 /* imm26 */

	case ADUFFZERO,
		ABL:
		return 1<<31 | 5<<26
	}

	ctxt.Diag("bad bra %v", Aconv(a))
	prasm(ctxt.Curp)
	return 0
}

func opbrr(ctxt *obj.Link, a int) uint32 {
	switch a {
	case ABL:
		return OPBLR(1) /* BLR */

	case AB:
		return OPBLR(0) /* BR */

	case ARET:
		return OPBLR(2) /* RET */
	}

	ctxt.Diag("bad brr %v", Aconv(a))
	prasm(ctxt.Curp)
	return 0
}

func op0(ctxt *obj.Link, a int) uint32 {
	switch a {
	case ADRPS:
		return 0x6B<<25 | 5<<21 | 0x1F<<16 | 0x1F<<5

	case AERET:
		return 0x6B<<25 | 4<<21 | 0x1F<<16 | 0<<10 | 0x1F<<5

	case ANOP:
		return SYSHINT(0)

	case AYIELD:
		return SYSHINT(1)

	case AWFE:
		return SYSHINT(2)

	case AWFI:
		return SYSHINT(3)

	case ASEV:
		return SYSHINT(4)

	case ASEVL:
		return SYSHINT(5)
	}

	ctxt.Diag("bad op0 %v", Aconv(a))
	prasm(ctxt.Curp)
	return 0
}

/*
 * register offset
 */
func opload(ctxt *obj.Link, a int) uint32 {

	switch a {
	case ALDAR:
		return LDSTX(3, 1, 1, 0, 1) | 0x1F<<10

	case ALDARW:
		return LDSTX(2, 1, 1, 0, 1) | 0x1F<<10

	case ALDARB:
		return LDSTX(0, 1, 1, 0, 1) | 0x1F<<10

	case ALDARH:
		return LDSTX(1, 1, 1, 0, 1) | 0x1F<<10

	case ALDAXP:
		return LDSTX(3, 0, 1, 1, 1)

	case ALDAXPW:
		return LDSTX(2, 0, 1, 1, 1)

	case ALDAXR:
		return LDSTX(3, 0, 1, 0, 1) | 0x1F<<10

	case ALDAXRW:
		return LDSTX(2, 0, 1, 0, 1) | 0x1F<<10

	case ALDAXRB:
		return LDSTX(0, 0, 1, 0, 1) | 0x1F<<10

	case ALDAXRH:
		return LDSTX(1, 0, 1, 0, 1) | 0x1F<<10

	case ALDXR:
		return LDSTX(3, 0, 1, 0, 0) | 0x1F<<10

	case ALDXRB:
		return LDSTX(0, 0, 1, 0, 0) | 0x1F<<10

	case ALDXRH:
		return LDSTX(1, 0, 1, 0, 0) | 0x1F<<10

	case ALDXRW:
		return LDSTX(2, 0, 1, 0, 0) | 0x1F<<10

	case ALDXP:
		return LDSTX(3, 0, 1, 1, 0)

	case ALDXPW:
		return LDSTX(2, 0, 1, 1, 0)

	case AMOVNP:
		return S64 | 0<<30 | 5<<27 | 0<<26 | 0<<23 | 1<<22

	case AMOVNPW:
		return S32 | 0<<30 | 5<<27 | 0<<26 | 0<<23 | 1<<22
	}

	ctxt.Diag("bad opload %v\n%v", Aconv(a), ctxt.Curp)
	return 0
}

func opstore(ctxt *obj.Link, a int) uint32 {
	switch a {
	case ASTLR:
		return LDSTX(3, 1, 0, 0, 1) | 0x1F<<10

	case ASTLRB:
		return LDSTX(0, 1, 0, 0, 1) | 0x1F<<10

	case ASTLRH:
		return LDSTX(1, 1, 0, 0, 1) | 0x1F<<10

	case ASTLP:
		return LDSTX(3, 0, 0, 1, 1)

	case ASTLPW:
		return LDSTX(2, 0, 0, 1, 1)

	case ASTLRW:
		return LDSTX(2, 1, 0, 0, 1) | 0x1F<<10

	case ASTLXP:
		return LDSTX(2, 0, 0, 1, 1)

	case ASTLXPW:
		return LDSTX(3, 0, 0, 1, 1)

	case ASTLXR:
		return LDSTX(3, 0, 0, 0, 1) | 0x1F<<10

	case ASTLXRB:
		return LDSTX(0, 0, 0, 0, 1) | 0x1F<<10

	case ASTLXRH:
		return LDSTX(1, 0, 0, 0, 1) | 0x1F<<10

	case ASTLXRW:
		return LDSTX(2, 0, 0, 0, 1) | 0x1F<<10

	case ASTXR:
		return LDSTX(3, 0, 0, 0, 0) | 0x1F<<10

	case ASTXRB:
		return LDSTX(0, 0, 0, 0, 0) | 0x1F<<10

	case ASTXRH:
		return LDSTX(1, 0, 0, 0, 0) | 0x1F<<10

	case ASTXP:
		return LDSTX(3, 0, 0, 1, 0)

	case ASTXPW:
		return LDSTX(2, 0, 0, 1, 0)

	case ASTXRW:
		return LDSTX(2, 0, 0, 0, 0) | 0x1F<<10

	case AMOVNP:
		return S64 | 0<<30 | 5<<27 | 0<<26 | 0<<23 | 1<<22

	case AMOVNPW:
		return S32 | 0<<30 | 5<<27 | 0<<26 | 0<<23 | 1<<22
	}

	ctxt.Diag("bad opstore %v\n%v", Aconv(a), ctxt.Curp)
	return 0
}

/*
 * load/store register (unsigned immediate) C3.3.13
 *	these produce 64-bit values (when there's an option)
 */
func olsr12u(ctxt *obj.Link, o int32, v int32, b int, r int) uint32 {

	if v < 0 || v >= (1<<12) {
		ctxt.Diag("offset out of range: %d\n%v", v, ctxt.Curp)
	}
	o |= (v & 0xFFF) << 10
	o |= int32(b) << 5
	o |= int32(r)
	return uint32(o)
}

func opldr12(ctxt *obj.Link, a int) uint32 {
	switch a {
	case AMOV:
		return LDSTR12U(3, 0, 1) /* imm12<<10 | Rn<<5 | Rt */

	case AMOVW:
		return LDSTR12U(2, 0, 2)

	case AMOVWU:
		return LDSTR12U(2, 0, 1)

	case AMOVH:
		return LDSTR12U(1, 0, 2)

	case AMOVHU:
		return LDSTR12U(1, 0, 1)

	case AMOVB:
		return LDSTR12U(0, 0, 2)

	case AMOVBU:
		return LDSTR12U(0, 0, 1)

	case AFMOVS:
		return LDSTR12U(2, 1, 1)

	case AFMOVD:
		return LDSTR12U(3, 1, 1)
	}

	ctxt.Diag("bad opldr12 %v\n%v", Aconv(a), ctxt.Curp)
	return 0
}

func opstr12(ctxt *obj.Link, a int) uint32 {
	return LD2STR(opldr12(ctxt, a))
}

/*
 * load/store register (unscaled immediate) C3.3.12
 */
func olsr9s(ctxt *obj.Link, o int32, v int32, b int, r int) uint32 {

	if v < -256 || v > 255 {
		ctxt.Diag("offset out of range: %d\n%v", v, ctxt.Curp)
	}
	o |= (v & 0x1FF) << 12
	o |= int32(b) << 5
	o |= int32(r)
	return uint32(o)
}

func opldr9(ctxt *obj.Link, a int) uint32 {
	switch a {
	case AMOV:
		return LDSTR9S(3, 0, 1) /* simm9<<12 | Rn<<5 | Rt */

	case AMOVW:
		return LDSTR9S(2, 0, 2)

	case AMOVWU:
		return LDSTR9S(2, 0, 1)

	case AMOVH:
		return LDSTR9S(1, 0, 2)

	case AMOVHU:
		return LDSTR9S(1, 0, 1)

	case AMOVB:
		return LDSTR9S(0, 0, 2)

	case AMOVBU:
		return LDSTR9S(0, 0, 1)

	case AFMOVS:
		return LDSTR9S(2, 1, 1)

	case AFMOVD:
		return LDSTR9S(3, 1, 1)
	}

	ctxt.Diag("bad opldr9 %v\n%v", Aconv(a), ctxt.Curp)
	return 0
}

func opstr9(ctxt *obj.Link, a int) uint32 {
	return LD2STR(opldr9(ctxt, a))
}

func opldrpp(ctxt *obj.Link, a int) uint32 {
	switch a {
	case AMOV:
		return 3<<30 | 7<<27 | 0<<26 | 0<<24 | 1<<22 /* simm9<<12 | Rn<<5 | Rt */

	case AMOVW:
		return 2<<30 | 7<<27 | 0<<26 | 0<<24 | 2<<22

	case AMOVWU:
		return 2<<30 | 7<<27 | 0<<26 | 0<<24 | 1<<22

	case AMOVH:
		return 1<<30 | 7<<27 | 0<<26 | 0<<24 | 2<<22

	case AMOVHU:
		return 1<<30 | 7<<27 | 0<<26 | 0<<24 | 1<<22

	case AMOVB:
		return 0<<30 | 7<<27 | 0<<26 | 0<<24 | 2<<22

	case AMOVBU:
		return 0<<30 | 7<<27 | 0<<26 | 0<<24 | 1<<22
	}

	ctxt.Diag("bad opldr %v\n%v", Aconv(a), ctxt.Curp)
	return 0
}

/*
 * load/store register (extended register)
 */
func olsxrr(ctxt *obj.Link, as int, rt int, r1 int, r2 int) uint32 {

	ctxt.Diag("need load/store extended register\n%v", ctxt.Curp)
	return 0xffffffff
}

func oaddi(o1 int32, v int32, r int, rt int) uint32 {
	if (v & 0xFFF000) != 0 {
		v >>= 12
		o1 |= 1 << 22
	}

	o1 |= ((v & 0xFFF) << 10) | (int32(r) << 5) | int32(rt)
	return uint32(o1)
}

/*
 * load a a literal value into dr
 */
func omovlit(ctxt *obj.Link, as int, p *obj.Prog, a *obj.Addr, dr int) uint32 {

	var v int32
	var o1 int32
	var w int
	var fp int
	if p.Pcond == nil { /* not in literal pool */
		aclass(ctxt, a)
		fmt.Fprintf(ctxt.Bso, "omovlit add %d (%#x)\n", ctxt.Instoffset, uint64(ctxt.Instoffset))

		/* TO DO: could be clever, and use general constant builder */
		o1 = int32(opirr(ctxt, AADD))

		v = int32(ctxt.Instoffset)
		if v != 0 && (v&0xFFF) == 0 {
			v >>= 12
			o1 |= 1 << 22 /* shift, by 12 */
		}

		o1 |= ((v & 0xFFF) << 10) | (REGZERO << 5) | int32(dr)
	} else {

		fp = 0
		w = 0 /* default: 32 bit, unsigned */
		switch as {
		case AFMOVS:
			fp = 1

		case AFMOVD:
			fp = 1
			w = 1 /* 64 bit simd&fp */

		case AMOV:
			if p.Pcond.As == ADWORD {
				w = 1 /* 64 bit */
			} else if p.Pcond.To.Offset < 0 {
				w = 2 /* sign extend */
			}

		case AMOVB,
			AMOVH,
			AMOVW:
			w = 2 /* 32 bit, sign-extended to 64 */
			break
		}

		v = int32(brdist(ctxt, p, 0, 19, 2))
		o1 = (int32(w) << 30) | (int32(fp) << 26) | (3 << 27)
		o1 |= (v & 0x7FFFF) << 5
		o1 |= int32(dr)
	}

	return uint32(o1)
}

func opbfm(ctxt *obj.Link, a int, r int, s int, rf int, rt int) uint32 {
	var o uint32
	var c uint32
	o = opirr(ctxt, a)
	if (o & (1 << 31)) == 0 {
		c = 32
	} else {

		c = 64
	}
	if r < 0 || uint32(r) >= c {
		ctxt.Diag("illegal bit number\n%v", ctxt.Curp)
	}
	o |= (uint32(r) & 0x3F) << 16
	if s < 0 || uint32(s) >= c {
		ctxt.Diag("illegal bit number\n%v", ctxt.Curp)
	}
	o |= (uint32(s) & 0x3F) << 10
	o |= (uint32(rf) << 5) | uint32(rt)
	return o
}

func opextr(ctxt *obj.Link, a int, v int32, rn int, rm int, rt int) uint32 {
	var o uint32
	var c uint32
	o = opirr(ctxt, a)
	if (o & (1 << 31)) != 0 {
		c = 63
	} else {

		c = 31
	}
	if v < 0 || uint32(v) > c {
		ctxt.Diag("illegal bit number\n%v", ctxt.Curp)
	}
	o |= uint32(v) << 10
	o |= uint32(rn) << 5
	o |= uint32(rm) << 16
	o |= uint32(rt)
	return o
}

/*
 * size in log2(bytes)
 */
func movesize(a int) int {

	switch a {
	case AMOV:
		return 3

	case AMOVW,
		AMOVWU:
		return 2

	case AMOVH,
		AMOVHU:
		return 1

	case AMOVB,
		AMOVBU:
		return 0

	case AFMOVS:
		return 2

	case AFMOVD:
		return 3

	default:
		return -1
	}
}