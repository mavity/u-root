// Copyright 2016-2020 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chmod

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	classUser uint8 = 1 << iota
	classGroup
	classOther
	classAll = classUser | classGroup | classOther
)

type modeExpression struct {
	octal   *uint16
	clauses []modeOperation
}

type modeOperation struct {
	classes     uint8
	whoOmitted  bool
	operator    byte
	permissions string
}

func parseModeExpression(input string) (modeExpression, error) {
	if input == "" {
		return modeExpression{}, fmt.Errorf("empty mode")
	}

	allOctal := true
	for _, r := range input {
		if r < '0' || r > '7' {
			allOctal = false
			break
		}
	}
	if allOctal {
		value, err := strconv.ParseUint(input, 8, 16)
		if err != nil || value > 0o7777 {
			return modeExpression{}, fmt.Errorf("%w: invalid octal mode %q: expected a value from 0000 through 7777",
				strconv.ErrRange, input)
		}
		mode := uint16(value)
		return modeExpression{octal: &mode}, nil
	}

	expr := modeExpression{}
	for i := 0; i < len(input); {
		whoStart := i
		var classes uint8
		for i < len(input) {
			switch input[i] {
			case 'u':
				classes |= classUser
			case 'g':
				classes |= classGroup
			case 'o':
				classes |= classOther
			case 'a':
				classes |= classAll
			default:
				goto whoDone
			}
			i++
		}
	whoDone:
		whoOmitted := i == whoStart
		if whoOmitted {
			classes = classAll
		}
		if i == len(input) || !isModeOperator(input[i]) {
			return modeExpression{}, fmt.Errorf("%w: invalid symbolic mode %q: expected an operator", strconv.ErrSyntax, input)
		}

		for {
			operator := input[i]
			i++
			permissionsStart := i
			for i < len(input) && !isModeOperator(input[i]) && input[i] != ',' {
				if !strings.ContainsRune("rwxXstugo", rune(input[i])) {
					return modeExpression{}, fmt.Errorf("%w: invalid symbolic mode %q: invalid permission %q",
						strconv.ErrSyntax, input, input[i:i+1])
				}
				i++
			}
			permissions := input[permissionsStart:i]
			if permissions == "" && operator != '=' {
				return modeExpression{}, fmt.Errorf("%w: invalid symbolic mode %q: empty permissions require '='",
					strconv.ErrSyntax, input)
			}
			if err := validatePermissions(input, permissions); err != nil {
				return modeExpression{}, err
			}
			expr.clauses = append(expr.clauses, modeOperation{
				classes:     classes,
				whoOmitted:  whoOmitted,
				operator:    operator,
				permissions: permissions,
			})

			if i == len(input) || input[i] == ',' {
				break
			}
		}

		if i < len(input) {
			i++
			if i == len(input) {
				return modeExpression{}, fmt.Errorf("%w: invalid symbolic mode %q: empty clause", strconv.ErrSyntax, input)
			}
		}
	}
	return expr, nil
}

func isModeOperator(b byte) bool {
	return b == '+' || b == '-' || b == '='
}

func validatePermissions(input, permissions string) error {
	copyPermissions := 0
	for i := range permissions {
		if strings.ContainsRune("ugo", rune(permissions[i])) {
			copyPermissions++
		}
	}
	if copyPermissions > 0 && (copyPermissions != 1 || len(permissions) != 1) {
		return fmt.Errorf("%w: invalid symbolic mode %q: a copied permission must be the only permission",
			strconv.ErrSyntax, input)
	}
	return nil
}

func (expr modeExpression) apply(current uint16, isDir bool, umask uint16) (uint16, bool) {
	if expr.octal != nil {
		return *expr.octal, false
	}

	original := current
	maskConflict := false
	for _, clause := range expr.clauses {
		permissions, special := clause.permissionBits(current, original, isDir)
		effectivePermissions := permissions
		if clause.whoOmitted {
			effectivePermissions &= 0o777 &^ (umask & 0o777)
			if clause.operator == '-' && current&permissions&umask&0o777 != 0 {
				maskConflict = true
			}
		}
		value := effectivePermissions | special

		switch clause.operator {
		case '+':
			current |= value
		case '-':
			current &^= value
		case '=':
			permissionMask := permissionMaskForClasses(clause.classes)
			specialMask := specialMaskForClasses(clause.classes)
			if clause.whoOmitted {
				permissionMask = 0o777
				specialMask = 0o7000
			}
			current &^= permissionMask | specialMask
			current |= value
		}
	}
	return current, maskConflict
}

func (op modeOperation) permissionBits(current, original uint16, isDir bool) (uint16, uint16) {
	var permissions, special uint16
	shiftForClass := func(class uint8) uint {
		switch class {
		case classUser:
			return 6
		case classGroup:
			return 3
		default:
			return 0
		}
	}
	for _, permission := range op.permissions {
		switch permission {
		case 'r', 'w', 'x', 'X':
			var bits uint16
			switch permission {
			case 'r':
				bits = 0o4
			case 'w':
				bits = 0o2
			case 'x':
				bits = 0o1
			case 'X':
				if isDir || original&0o111 != 0 {
					bits = 0o1
				}
			}
			for _, class := range []uint8{classUser, classGroup, classOther} {
				if op.classes&class != 0 {
					permissions |= bits << shiftForClass(class)
				}
			}
		case 's':
			if op.classes&classUser != 0 {
				special |= 0o4000
			}
			if op.classes&classGroup != 0 {
				special |= 0o2000
			}
		case 't':
			if op.classes&classOther != 0 {
				special |= 0o1000
			}
		case 'u', 'g', 'o':
			sourceClass := classUser
			if permission == 'g' {
				sourceClass = classGroup
			} else if permission == 'o' {
				sourceClass = classOther
			}
			sourceBits := (current >> shiftForClass(sourceClass)) & 0o7
			for _, class := range []uint8{classUser, classGroup, classOther} {
				if op.classes&class != 0 {
					permissions |= sourceBits << shiftForClass(class)
				}
			}
		}
	}
	return permissions, special
}

func permissionMaskForClasses(classes uint8) uint16 {
	var mask uint16
	if classes&classUser != 0 {
		mask |= 0o700
	}
	if classes&classGroup != 0 {
		mask |= 0o070
	}
	if classes&classOther != 0 {
		mask |= 0o007
	}
	return mask
}

func specialMaskForClasses(classes uint8) uint16 {
	var mask uint16
	if classes&classUser != 0 {
		mask |= 0o4000
	}
	if classes&classGroup != 0 {
		mask |= 0o2000
	}
	if classes&classOther != 0 {
		mask |= 0o1000
	}
	return mask
}
