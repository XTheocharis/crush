; Definitions

(class_declaration
  (identifier) @name.definition.class) @definition.class

(function_declaration
  (identifier) @name.definition.function) @definition.function

(object_declaration
  (identifier) @name.definition.object) @definition.object

; References

(call_expression
  (identifier) @name.reference.call) @reference.call
