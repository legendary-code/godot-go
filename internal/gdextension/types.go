package gdextension

/*
#cgo CFLAGS: -I${SRCDIR} -I${SRCDIR}/../../godot

#include "shim.h"
*/
import "C"

import "unsafe"

// InitializationLevel mirrors GDExtensionInitializationLevel.
type InitializationLevel int32

const (
	InitLevelCore    InitializationLevel = C.GDEXTENSION_INITIALIZATION_CORE
	InitLevelServers InitializationLevel = C.GDEXTENSION_INITIALIZATION_SERVERS
	InitLevelScene   InitializationLevel = C.GDEXTENSION_INITIALIZATION_SCENE
	InitLevelEditor  InitializationLevel = C.GDEXTENSION_INITIALIZATION_EDITOR
)

// MaxInitLevel matches GDEXTENSION_MAX_INITIALIZATION_LEVEL — the count of
// distinct initialization levels (one greater than the last valid level).
const MaxInitLevel = int32(C.GDEXTENSION_MAX_INITIALIZATION_LEVEL)

func (l InitializationLevel) String() string {
	switch l {
	case InitLevelCore:
		return "core"
	case InitLevelServers:
		return "servers"
	case InitLevelScene:
		return "scene"
	case InitLevelEditor:
		return "editor"
	default:
		return "unknown"
	}
}

// VariantType mirrors GDExtensionVariantType.
type VariantType int32

const (
	VariantTypeNil                VariantType = C.GDEXTENSION_VARIANT_TYPE_NIL
	VariantTypeBool               VariantType = C.GDEXTENSION_VARIANT_TYPE_BOOL
	VariantTypeInt                VariantType = C.GDEXTENSION_VARIANT_TYPE_INT
	VariantTypeFloat              VariantType = C.GDEXTENSION_VARIANT_TYPE_FLOAT
	VariantTypeString             VariantType = C.GDEXTENSION_VARIANT_TYPE_STRING
	VariantTypeVector2            VariantType = C.GDEXTENSION_VARIANT_TYPE_VECTOR2
	VariantTypeVector2i           VariantType = C.GDEXTENSION_VARIANT_TYPE_VECTOR2I
	VariantTypeRect2              VariantType = C.GDEXTENSION_VARIANT_TYPE_RECT2
	VariantTypeRect2i             VariantType = C.GDEXTENSION_VARIANT_TYPE_RECT2I
	VariantTypeVector3            VariantType = C.GDEXTENSION_VARIANT_TYPE_VECTOR3
	VariantTypeVector3i           VariantType = C.GDEXTENSION_VARIANT_TYPE_VECTOR3I
	VariantTypeTransform2D        VariantType = C.GDEXTENSION_VARIANT_TYPE_TRANSFORM2D
	VariantTypeVector4            VariantType = C.GDEXTENSION_VARIANT_TYPE_VECTOR4
	VariantTypeVector4i           VariantType = C.GDEXTENSION_VARIANT_TYPE_VECTOR4I
	VariantTypePlane              VariantType = C.GDEXTENSION_VARIANT_TYPE_PLANE
	VariantTypeQuaternion         VariantType = C.GDEXTENSION_VARIANT_TYPE_QUATERNION
	VariantTypeAABB               VariantType = C.GDEXTENSION_VARIANT_TYPE_AABB
	VariantTypeBasis              VariantType = C.GDEXTENSION_VARIANT_TYPE_BASIS
	VariantTypeTransform3D        VariantType = C.GDEXTENSION_VARIANT_TYPE_TRANSFORM3D
	VariantTypeProjection         VariantType = C.GDEXTENSION_VARIANT_TYPE_PROJECTION
	VariantTypeColor              VariantType = C.GDEXTENSION_VARIANT_TYPE_COLOR
	VariantTypeStringName         VariantType = C.GDEXTENSION_VARIANT_TYPE_STRING_NAME
	VariantTypeNodePath           VariantType = C.GDEXTENSION_VARIANT_TYPE_NODE_PATH
	VariantTypeRID                VariantType = C.GDEXTENSION_VARIANT_TYPE_RID
	VariantTypeObject             VariantType = C.GDEXTENSION_VARIANT_TYPE_OBJECT
	VariantTypeCallable           VariantType = C.GDEXTENSION_VARIANT_TYPE_CALLABLE
	VariantTypeSignal             VariantType = C.GDEXTENSION_VARIANT_TYPE_SIGNAL
	VariantTypeDictionary         VariantType = C.GDEXTENSION_VARIANT_TYPE_DICTIONARY
	VariantTypeArray              VariantType = C.GDEXTENSION_VARIANT_TYPE_ARRAY
	VariantTypePackedByteArray    VariantType = C.GDEXTENSION_VARIANT_TYPE_PACKED_BYTE_ARRAY
	VariantTypePackedInt32Array   VariantType = C.GDEXTENSION_VARIANT_TYPE_PACKED_INT32_ARRAY
	VariantTypePackedInt64Array   VariantType = C.GDEXTENSION_VARIANT_TYPE_PACKED_INT64_ARRAY
	VariantTypePackedFloat32Array VariantType = C.GDEXTENSION_VARIANT_TYPE_PACKED_FLOAT32_ARRAY
	VariantTypePackedFloat64Array VariantType = C.GDEXTENSION_VARIANT_TYPE_PACKED_FLOAT64_ARRAY
	VariantTypePackedStringArray  VariantType = C.GDEXTENSION_VARIANT_TYPE_PACKED_STRING_ARRAY
	VariantTypePackedVector2Array VariantType = C.GDEXTENSION_VARIANT_TYPE_PACKED_VECTOR2_ARRAY
	VariantTypePackedVector3Array VariantType = C.GDEXTENSION_VARIANT_TYPE_PACKED_VECTOR3_ARRAY
	VariantTypePackedColorArray   VariantType = C.GDEXTENSION_VARIANT_TYPE_PACKED_COLOR_ARRAY
	VariantTypePackedVector4Array VariantType = C.GDEXTENSION_VARIANT_TYPE_PACKED_VECTOR4_ARRAY
	VariantTypeMax                VariantType = C.GDEXTENSION_VARIANT_TYPE_VARIANT_MAX
)

// VariantOperator mirrors GDExtensionVariantOperator.
type VariantOperator int32

const (
	OpEqual        VariantOperator = C.GDEXTENSION_VARIANT_OP_EQUAL
	OpNotEqual     VariantOperator = C.GDEXTENSION_VARIANT_OP_NOT_EQUAL
	OpLess         VariantOperator = C.GDEXTENSION_VARIANT_OP_LESS
	OpLessEqual    VariantOperator = C.GDEXTENSION_VARIANT_OP_LESS_EQUAL
	OpGreater      VariantOperator = C.GDEXTENSION_VARIANT_OP_GREATER
	OpGreaterEqual VariantOperator = C.GDEXTENSION_VARIANT_OP_GREATER_EQUAL
	OpAdd          VariantOperator = C.GDEXTENSION_VARIANT_OP_ADD
	OpSubtract     VariantOperator = C.GDEXTENSION_VARIANT_OP_SUBTRACT
	OpMultiply     VariantOperator = C.GDEXTENSION_VARIANT_OP_MULTIPLY
	OpDivide       VariantOperator = C.GDEXTENSION_VARIANT_OP_DIVIDE
	OpNegate       VariantOperator = C.GDEXTENSION_VARIANT_OP_NEGATE
	OpPositive     VariantOperator = C.GDEXTENSION_VARIANT_OP_POSITIVE
	OpModule       VariantOperator = C.GDEXTENSION_VARIANT_OP_MODULE
	OpPower        VariantOperator = C.GDEXTENSION_VARIANT_OP_POWER
	OpShiftLeft    VariantOperator = C.GDEXTENSION_VARIANT_OP_SHIFT_LEFT
	OpShiftRight   VariantOperator = C.GDEXTENSION_VARIANT_OP_SHIFT_RIGHT
	OpBitAnd       VariantOperator = C.GDEXTENSION_VARIANT_OP_BIT_AND
	OpBitOr        VariantOperator = C.GDEXTENSION_VARIANT_OP_BIT_OR
	OpBitXor       VariantOperator = C.GDEXTENSION_VARIANT_OP_BIT_XOR
	OpBitNegate    VariantOperator = C.GDEXTENSION_VARIANT_OP_BIT_NEGATE
	OpAnd          VariantOperator = C.GDEXTENSION_VARIANT_OP_AND
	OpOr           VariantOperator = C.GDEXTENSION_VARIANT_OP_OR
	OpXor          VariantOperator = C.GDEXTENSION_VARIANT_OP_XOR
	OpNot          VariantOperator = C.GDEXTENSION_VARIANT_OP_NOT
	OpIn           VariantOperator = C.GDEXTENSION_VARIANT_OP_IN
	OpMax          VariantOperator = C.GDEXTENSION_VARIANT_OP_MAX
)

// CallErrorType mirrors GDExtensionCallErrorType.
type CallErrorType int32

const (
	CallErrorOK                 CallErrorType = C.GDEXTENSION_CALL_OK
	CallErrorInvalidMethod      CallErrorType = C.GDEXTENSION_CALL_ERROR_INVALID_METHOD
	CallErrorInvalidArgument    CallErrorType = C.GDEXTENSION_CALL_ERROR_INVALID_ARGUMENT
	CallErrorTooManyArguments   CallErrorType = C.GDEXTENSION_CALL_ERROR_TOO_MANY_ARGUMENTS
	CallErrorTooFewArguments    CallErrorType = C.GDEXTENSION_CALL_ERROR_TOO_FEW_ARGUMENTS
	CallErrorInstanceIsNull     CallErrorType = C.GDEXTENSION_CALL_ERROR_INSTANCE_IS_NULL
	CallErrorMethodNotConst     CallErrorType = C.GDEXTENSION_CALL_ERROR_METHOD_NOT_CONST
)

// MethodFlags mirrors GDExtensionClassMethodFlags.
type MethodFlags uint32

const (
	MethodFlagNormal          MethodFlags = C.GDEXTENSION_METHOD_FLAG_NORMAL
	MethodFlagEditor          MethodFlags = C.GDEXTENSION_METHOD_FLAG_EDITOR
	MethodFlagConst           MethodFlags = C.GDEXTENSION_METHOD_FLAG_CONST
	MethodFlagVirtual         MethodFlags = C.GDEXTENSION_METHOD_FLAG_VIRTUAL
	MethodFlagVararg          MethodFlags = C.GDEXTENSION_METHOD_FLAG_VARARG
	MethodFlagStatic          MethodFlags = C.GDEXTENSION_METHOD_FLAG_STATIC
	MethodFlagVirtualRequired MethodFlags = C.GDEXTENSION_METHOD_FLAG_VIRTUAL_REQUIRED
	MethodFlagsDefault        MethodFlags = C.GDEXTENSION_METHOD_FLAGS_DEFAULT
)

// MethodArgumentMetadata mirrors GDExtensionClassMethodArgumentMetadata.
type MethodArgumentMetadata int32

const (
	ArgMetaNone             MethodArgumentMetadata = C.GDEXTENSION_METHOD_ARGUMENT_METADATA_NONE
	ArgMetaIntIsInt8        MethodArgumentMetadata = C.GDEXTENSION_METHOD_ARGUMENT_METADATA_INT_IS_INT8
	ArgMetaIntIsInt16       MethodArgumentMetadata = C.GDEXTENSION_METHOD_ARGUMENT_METADATA_INT_IS_INT16
	ArgMetaIntIsInt32       MethodArgumentMetadata = C.GDEXTENSION_METHOD_ARGUMENT_METADATA_INT_IS_INT32
	ArgMetaIntIsInt64       MethodArgumentMetadata = C.GDEXTENSION_METHOD_ARGUMENT_METADATA_INT_IS_INT64
	ArgMetaIntIsUint8       MethodArgumentMetadata = C.GDEXTENSION_METHOD_ARGUMENT_METADATA_INT_IS_UINT8
	ArgMetaIntIsUint16      MethodArgumentMetadata = C.GDEXTENSION_METHOD_ARGUMENT_METADATA_INT_IS_UINT16
	ArgMetaIntIsUint32      MethodArgumentMetadata = C.GDEXTENSION_METHOD_ARGUMENT_METADATA_INT_IS_UINT32
	ArgMetaIntIsUint64      MethodArgumentMetadata = C.GDEXTENSION_METHOD_ARGUMENT_METADATA_INT_IS_UINT64
	ArgMetaRealIsFloat      MethodArgumentMetadata = C.GDEXTENSION_METHOD_ARGUMENT_METADATA_REAL_IS_FLOAT
	ArgMetaRealIsDouble     MethodArgumentMetadata = C.GDEXTENSION_METHOD_ARGUMENT_METADATA_REAL_IS_DOUBLE
	ArgMetaIntIsChar16      MethodArgumentMetadata = C.GDEXTENSION_METHOD_ARGUMENT_METADATA_INT_IS_CHAR16
	ArgMetaIntIsChar32      MethodArgumentMetadata = C.GDEXTENSION_METHOD_ARGUMENT_METADATA_INT_IS_CHAR32
	ArgMetaObjectIsRequired MethodArgumentMetadata = C.GDEXTENSION_METHOD_ARGUMENT_METADATA_OBJECT_IS_REQUIRED
)

// PropertyHint mirrors the global PropertyHint enum from
// extension_api.json. Values are sourced from the JSON, not from the
// gdextension_interface.h header (the header just declares the field
// type as uint32). Phase 6 surfaces the hints reachable through the
// codegen `@export_*` doctags; the rest are still usable via direct
// ClassPropertyDef construction in user code.
type PropertyHint uint32

const (
	PropertyHintNone            PropertyHint = 0
	PropertyHintRange           PropertyHint = 1
	PropertyHintEnum            PropertyHint = 2
	PropertyHintFile            PropertyHint = 13
	PropertyHintDir             PropertyHint = 14
	PropertyHintMultilineText   PropertyHint = 18
	PropertyHintPlaceholderText PropertyHint = 20
)

// Opaque pointer aliases. They carry no compile-time type safety on the C side
// (Godot's API is in C), but on the Go side they document intent and let us
// catch mix-ups (e.g. passing a Variant where a String is expected).
type (
	VariantPtr    unsafe.Pointer
	StringPtr     unsafe.Pointer
	StringNamePtr unsafe.Pointer
	NodePathPtr   unsafe.Pointer
	ObjectPtr     unsafe.Pointer
	TypePtr       unsafe.Pointer
	MethodBindPtr unsafe.Pointer

	// LibraryPtr identifies this loaded extension to the host. Carried in every
	// classdb_register_extension_class* call we'll make later.
	LibraryPtr unsafe.Pointer

	// ObjectInstanceID mirrors GDObjectInstanceID — the 64-bit handle that
	// references a Godot Object across script-engine boundaries.
	ObjectInstanceID uint64
)

// Resolved-function-pointer aliases. The host hands back C function pointers
// from getters like variant_get_ptr_constructor; we shuttle them through Go
// as opaque pointer-sized values and route them back into typed C trampolines
// for invocation. C permits casts between function pointer types, so the
// round-trip is well-defined; the Go-named types only document intent.
type (
	PtrConstructor       unsafe.Pointer
	PtrDestructor        unsafe.Pointer
	PtrBuiltInMethod     unsafe.Pointer
	PtrOperatorEvaluator unsafe.Pointer
	PtrIndexedSetter     unsafe.Pointer
	PtrIndexedGetter     unsafe.Pointer
	PtrKeyedSetter       unsafe.Pointer
	PtrKeyedGetter       unsafe.Pointer
	PtrUtilityFunction   unsafe.Pointer

	// VariantFromTypeFunc converts a typed value pointer into a Variant slot.
	VariantFromTypeFunc unsafe.Pointer
	// VariantToTypeFunc converts a Variant slot into a typed value pointer.
	VariantToTypeFunc unsafe.Pointer
)
