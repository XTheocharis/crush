; Class definition
(class_definition
  name: (identifier) @name.definition.class) @definition.class

; Function definition with optional return type
(function_definition
  name: (identifier) @name.definition.function
  parameters: (parameters) @params
  return_type: (type)? @return_type) @definition.function

; Call references
(call
  function: [
    (identifier) @name.reference.call
    (attribute
      attribute: (identifier) @name.reference.call)
  ]) @reference.call
