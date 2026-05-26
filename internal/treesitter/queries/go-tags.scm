(
  (comment)* @doc
  .
  (function_declaration
    name: (identifier) @name.definition.function
    parameters: (parameter_list) @params
    result: [
      (type_identifier) @return_type
      (pointer_type) @return_type
      (slice_type) @return_type
      (array_type) @return_type
      (interface_type) @return_type
      (channel_type) @return_type
      (map_type) @return_type
      (function_type) @return_type
      (qualified_type) @return_type
      (struct_type) @return_type
    ]?
  ) @definition.function
  (#strip! @doc "^//\\s*")
  (#set-adjacent! @doc @definition.function)
)

(
  (comment)* @doc
  .
  (method_declaration
    receiver: (parameter_list
      (parameter_declaration
        type: [
          (pointer_type (type_identifier))
          (type_identifier)
        ])) @parent
    name: (field_identifier) @name.definition.method
    parameters: (parameter_list) @params
    result: [
      (type_identifier) @return_type
      (pointer_type) @return_type
      (slice_type) @return_type
      (array_type) @return_type
      (interface_type) @return_type
      (channel_type) @return_type
      (map_type) @return_type
      (function_type) @return_type
      (qualified_type) @return_type
      (struct_type) @return_type
    ]?
  ) @definition.method
  (#strip! @doc "^//\\s*")
  (#set-adjacent! @doc @definition.method)
)

(call_expression
  function: [
    (identifier) @name.reference.call
    (parenthesized_expression (identifier) @name.reference.call)
    (selector_expression field: (field_identifier) @name.reference.call)
    (parenthesized_expression (selector_expression field: (field_identifier) @name.reference.call))
  ]) @reference.call

(
  (comment)* @doc
  .
  (type_declaration
    (type_spec
      name: (type_identifier) @name.definition.interface
      type: (interface_type)))
  @definition.interface
  (#strip! @doc "^//\\s*")
  (#set-adjacent! @doc @definition.interface)
)

(
  (comment)* @doc
  .
  (type_declaration
    (type_spec
      name: (type_identifier) @name.definition.class
      type: (struct_type)))
  @definition.class
  (#strip! @doc "^//\\s*")
  (#set-adjacent! @doc @definition.class)
)

(type_spec
  name: (type_identifier) @name.definition.type) @definition.type

(type_identifier) @name.reference.type @reference.type

(package_clause "package" (package_identifier) @name.definition.module)

(import_declaration (import_spec) @name.reference.module)

(var_declaration (var_spec name: (identifier) @name.definition.variable))

(const_declaration (const_spec name: (identifier) @name.definition.constant))
