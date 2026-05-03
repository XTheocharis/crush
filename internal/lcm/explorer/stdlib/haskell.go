package stdlib

import "strings"

var haskellModules = toSet([]string{
	"Control.Applicative", "Control.Arrow", "Control.Category",
	"Control.Concurrent", "Control.Exception", "Control.Monad",
	"Control.Monad.Fix", "Control.Monad.IO.Class",
	"Control.Monad.Reader", "Control.Monad.State", "Control.Monad.Writer",
	"Data.Bits", "Data.Bool", "Data.Char", "Data.Data",
	"Data.Dynamic", "Data.Either", "Data.Eq", "Data.Foldable",
	"Data.Function", "Data.Functor", "Data.Functor.Classes",
	"Data.IORef", "Data.Int", "Data.List", "Data.Map",
	"Data.Maybe", "Data.Monoid", "Data.Ord", "Data.Ratio",
	"Data.Set", "Data.String", "Data.Traversable", "Data.Tuple",
	"Data.Typeable", "Data.Word", "Foreign.C", "Foreign.Ptr",
	"Foreign.StablePtr", "Foreign.Storable", "GHC.Conc",
	"GHC.Err", "GHC.Exts", "GHC.IO", "GHC.IO.Handle",
	"Numeric", "Prelude", "System.Directory",
	"System.Environment", "System.Exit", "System.IO",
	"System.Posix", "System.Process", "Text.Read",
	"Text.Printf", "Text.Show",
})

func IsHaskellStdlib(module string) bool {
	return haskellModules[strings.TrimSpace(module)]
}
