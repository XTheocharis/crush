; --- Function signatures (interface/abstract) ---

(function_signature
  name: (identifier) @name.definition.function
  parameters: (formal_parameters) @params
  return_type: (type_annotation)? @return_type) @definition.function

(method_signature
  name: (property_identifier) @name.definition.method
  parameters: (formal_parameters) @params
  return_type: (type_annotation)? @return_type) @definition.method

(abstract_method_signature
  name: (property_identifier) @name.definition.method
  parameters: (formal_parameters) @params
  return_type: (type_annotation)? @return_type) @definition.method

; --- Declarations with doc comments ---

(
  (comment)* @doc
  .
  (function_declaration
    name: (identifier) @name.definition.function
    parameters: (formal_parameters) @params
    return_type: (type_annotation)? @return_type
  ) @definition.function
)

(
  (comment)* @doc
  .
  (method_definition
    name: (property_identifier) @name.definition.method
    parameters: (formal_parameters) @params
    return_type: (type_annotation)? @return_type
  ) @definition.method
)

(
  (comment)* @doc
  .
  (class_declaration
    name: (type_identifier) @name.definition.class
  ) @definition.class
)

(
  (comment)* @doc
  .
  (abstract_class_declaration
    name: (type_identifier) @name.definition.class
  ) @definition.class
)

; --- Other declarations ---

(module
  name: (identifier) @name.definition.module) @definition.module

(interface_declaration
  name: (type_identifier) @name.definition.interface) @definition.interface

(type_alias_declaration
  name: (type_identifier) @name.definition.type) @definition.type

(enum_declaration
  name: (identifier) @name.definition.enum) @definition.enum

; --- References ---

(type_annotation
  (type_identifier) @name.reference.type) @reference.type

(new_expression
  constructor: (identifier) @name.reference.class) @reference.class
