;; Julia tags query for tree-sitter-julia v0.25.0
;; License: MIT

;; Module definitions
(module_definition
  name: (identifier) @name.definition.module) @definition.module

;; Struct definitions (immutable and mutable share struct_definition node)
(struct_definition
  (type_head
    (identifier) @name.definition.class)) @definition.class

;; Abstract type definitions
(abstract_definition
  (type_head
    (identifier) @name.definition.class)) @definition.class

;; Constant definitions
(const_statement
  (assignment
    (identifier) @name.definition.constant)) @definition.constant

;; Function definitions (includes methods in v0.25.0)
(function_definition
  (signature
    (call_expression
      (identifier) @name.definition.function))) @definition.function

(function_definition
  (signature
    (identifier) @name.definition.function)) @definition.function

;; Short-form function definitions (assignment with call_expression on left)
(assignment
  (call_expression
    (identifier) @name.definition.function)) @definition.function

;; Macro definitions
(macro_definition
  (signature
    (call_expression
      (identifier) @name.definition.macro))) @definition.macro

;; Macro calls
(macrocall_expression
  (macro_identifier) @name.reference.call) @reference.call

;; Function/method calls
(call_expression
  (identifier) @name.reference.call) @reference.call

(call_expression
  (field_expression) @name.reference.call) @reference.call

;; Export statements
(export_statement
  (identifier) @name.reference.export) @reference.export

;; Using statements
(using_statement
  (identifier) @name.reference.module) @reference.module

;; Import statements
(import_statement
  (identifier) @name.reference.module) @reference.module
