package stdlib

import "strings"

var csharpNamespaces = toSet([]string{
	"System", "System.CodeDom", "System.CodeDom.Compiler",
	"System.Collections", "System.Collections.Concurrent",
	"System.Collections.Generic", "System.Collections.Immutable",
	"System.Collections.Specialized", "System.ComponentModel",
	"System.ComponentModel.DataAnnotations", "System.ComponentModel.Design",
	"System.ComponentModel.Primitives", "System.ComponentModel.TypeConverter",
	"System.Configuration", "System.Data", "System.Data.Common",
	"System.Data.Odbc", "System.Data.OleDb", "System.Data.SqlClient",
	"System.Diagnostics", "System.Diagnostics.CodeAnalysis",
	"System.Diagnostics.Contracts", "System.Diagnostics.Debug",
	"System.Diagnostics.EventLog", "System.Diagnostics.PerformanceData",
	"System.Diagnostics.Process", "System.Diagnostics.StackTrace",
	"System.Diagnostics.SymbolStore", "System.Diagnostics.Tools",
	"System.Diagnostics.Tracing", "System.Drawing", "System.Drawing.Drawing2D",
	"System.Drawing.Imaging", "System.Drawing.Printing", "System.Drawing.Text",
	"System.Dynamic", "System.Globalization", "System.IO",
	"System.IO.Compression", "System.IO.Compression.FileSystem",
	"System.IO.Compression.ZipFile", "System.IO.IsolatedStorage",
	"System.IO.MemoryMappedFiles", "System.IO.Packaging",
	"System.IO.Pipes", "System.IO.Ports", "System.Linq",
	"System.Linq.Expressions", "System.Linq.Parallel", "System.Linq.Queryable",
	"System.Management", "System.Management.Instrumentation",
	"System.Media", "System.Net", "System.Net.Cache", "System.Net.Http",
	"System.Net.Mail", "System.Net.Mime", "System.Net.NetworkInformation",
	"System.Net.PeerToPeer", "System.Net.PeerToPeer.Collaboration",
	"System.Net.Security", "System.Net.Sockets", "System.Net.WebSockets",
	"System.Numerics", "System.Reflection", "System.Reflection.Context",
	"System.Reflection.Emit", "System.Reflection.Emit.ILGenerator",
	"System.Reflection.Emit.Lightweight", "System.Reflection.Metadata",
	"System.Resources", "System.Resources.Extensions",
	"System.Resources.NetStandard", "System.Runtime",
	"System.Runtime.CompilerServices", "System.Runtime.ConstrainedExecution",
	"System.Runtime.Caching", "System.Runtime.ExceptionServices",
	"System.Runtime.Extensions", "System.Runtime.Handles",
	"System.Runtime.Hosting", "System.Runtime.InteropServices",
	"System.Runtime.InteropServices.ComTypes", "System.Runtime.InteropServices.WindowsRuntime",
	"System.Runtime.InteropServices.Marshalling", "System.Runtime.Intrinsics",
	"System.Runtime.Intrinsics.Arm", "System.Runtime.Intrinsics.X86",
	"System.Runtime.Loader", "System.Runtime.Remoting",
	"System.Runtime.Remoting.Activation", "System.Runtime.Remoting.Channels",
	"System.Runtime.Remoting.Channels.Ipc", "System.Runtime.Remoting.Channels.Tcp",
	"System.Runtime.Remoting.Contexts", "System.Runtime.Remoting.Lifetime",
	"System.Runtime.Remoting.Messaging", "System.Runtime.Remoting.Metadata",
	"System.Runtime.Remoting.Metadata.W3cXsd2001",
	"System.Runtime.Remoting.Proxies", "System.Runtime.Remoting.Services",
	"System.Runtime.Serialization", "System.Runtime.Serialization.Formatters",
	"System.Runtime.Serialization.Formatters.Binary",
	"System.Runtime.Serialization.Formatters.Soap",
	"System.Runtime.Serialization.Json", "System.Security",
	"System.Security.AccessControl", "System.Security.Authentication",
	"System.Security.Authentication.ExtendedProtection",
	"System.Security.Claims", "System.Security.Cryptography",
	"System.Security.Cryptography.X509Certificates", "Microsoft.Win32.Registry",
})

func IsCSharpStdlib(namespace string) bool {
	namespace = strings.TrimSpace(namespace)
	if csharpNamespaces[namespace] {
		return true
	}
	prefix := strings.Split(namespace, ".")[0]
	return csharpNamespaces[prefix+"."] || csharpNamespaces[prefix]
}
