(** A sample OCaml interface file for tag extraction testing. *)

module Config : sig
  type t

  val create : unit -> t
  val name : t -> string
end

module type Storage = sig
  type key
  type value

  val get : key -> value option
  val set : key -> value -> unit
end

class virtual base_handler : object
  method virtual handle : string -> unit
end

type color =
  | Red
  | Green
  | Blue

type point = {
  x : float;
  y : float;
}

external hash : string -> int = "caml_hash"

val run : unit -> unit
