package stdlib

import "strings"

var swiftModules = toSet([]string{
	"Swift", "AppKit", "CloudKit", "Combine", "CoreData",
	"CoreGraphics", "CoreImage", "CoreMotion", "CryptoKit",
	"Foundation", "HealthKit", "MapKit", "Metal",
	"MetalKit", "Network", "PassKit", "Photos",
	"PDFKit", "QuartzCore", "SceneKit", "SpriteKit",
	"SwiftUI", "UIKit", "UserNotifications", "Vision",
	"Accelerate", "AVFoundation", "AudioToolbox", "AudioUnit",
	"Charts", "Compression", "CoreAudio", "CoreAudioKit",
	"CoreBluetooth", "CoreData.NSSQLiteError", "CoreFoundation",
	"CoreHaptics", "CoreImage.CIFilterBuiltins", "CoreLocation",
	"CoreML", "CoreMIDI", "CoreMedia", "CoreMediaIO",
	"CoreMotion", "CoreML", "CoreNFC", "CoreSpotlight",
	"CoreTelephony", "CoreText", "CoreVideo", "Dispatch",
	"EventKit", "EventKitUI", "FileProvider", "GameplayKit",
	"GameKit", "GRDB", "Intents", "IntentsUI",
	"LocalAuthentication", "MachineLearning", "MapKit", "MediaPlayer",
	"MessageUI", "MultipeerConnectivity", "NaturalLanguage",
	"NearbyInteraction", "Network", "OSLog", "PencilKit",
	"PhotosUI", "PushKit", "QuartzCore", "ReplayKit",
	"SCNetworkReachability", "SceneKit", "Signal", "Speech",
	"SpriteKit", "StoreKit", "SwiftData", "SwiftUI",
	"SymptomPresentationFramework", "Swift", "SystemConfiguration",
	"UserNotifications", "VisionKit", "WatchConnectivity",
	"WatchKit", "WebKit",
})

func IsSwiftStdlib(module string) bool {
	module = strings.TrimSpace(module)
	if swiftModules[module] {
		return true
	}
	prefix := strings.Split(module, ".")[0]
	return swiftModules[prefix]
}
