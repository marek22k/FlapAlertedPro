//go:build !core_doubleAddPath

package monitor

import (
	"FlapAlertedPro/bgp"
	"log"
	"math"
	"sync"
	"sync/atomic"
	"time"
)

var (
	GlobalPerPeerState bool   = false
	GlobalNotifyOnce   bool   = false
	GlobalKeepPathInfo bool   = true
	FlapPeriod         int64  = 2
	NotifyTarget       uint64 = 10
	GlobalDeepCopy     bool   = true
)

const PathLimit = 1000

type Flap struct {
	Cidr                 string
	LastPath             map[uint32]bgp.AsPath
	Paths                []bgp.AsPath
	PathChangeCount      uint64
	PathChangeCountTotal uint64
	FirstSeen            int64
	LastSeen             int64
}

func StartMonitoring(asn uint32, flapPeriod int64, notifyTarget uint64, addPath bool, perPeerState bool, debug bool, notifyOnce bool, keepPathInfo bool) {
	FlapPeriod = flapPeriod
	NotifyTarget = notifyTarget
	bgp.GlobalAddPath = addPath
	GlobalPerPeerState = perPeerState
	bgp.GlobalDebug = debug
	GlobalNotifyOnce = notifyOnce
	GlobalKeepPathInfo = keepPathInfo

	updateChannel := make(chan *bgp.UserUpdate, 11000)
	// Initialize activeFlapList and flapMap
	flapMap = make(map[string]*Flap)
	activeFlapList = make([]*Flap, 0)

	go processUpdates(updateChannel)
	go moduleCallback()
	go bgp.StartBGP(asn, updateChannel)
	cleanUpFlapList()
}

var (
	flapMap          map[string]*Flap
	flapMapMu        sync.RWMutex
	activeFlapList   []*Flap
	activeFlapListMu sync.RWMutex
)

func processUpdates(updateChannel chan *bgp.UserUpdate) {
	for {
		update := <-updateChannel
		if update == nil {
			return
		}

		if len(updateChannel) > 10700 {
			if atomic.CompareAndSwapInt32(&updateDropperRunning, int32(0), int32(1)) {
				go updateDropper(updateChannel)
			}
		}

		flapMapMu.Lock()
		for i := range update.Prefix {
			updateList(update.Prefix[i], update.Path)
		}
		flapMapMu.Unlock()
	}
}

func cleanUpFlapList() {
	for {
		select {
		case <-time.After(1 * time.Duration(FlapPeriod) * time.Second):
		}

		currentTime := time.Now().Unix()
		flapMapMu.Lock()
		activeFlapListMu.Lock()
		for key, element := range flapMap {
			if element.LastSeen+FlapPeriod <= currentTime {
				delete(flapMap, key)

				var activeIndex int
				var found = false
				for i := range activeFlapList {
					if activeFlapList[i].Cidr == element.Cidr {
						activeIndex = i
						found = true
					}
				}
				if found {
					activeFlapList[activeIndex] = activeFlapList[len(activeFlapList)-1]
					activeFlapList = activeFlapList[:len(activeFlapList)-1]
				}

			}
		}
		flapMapMu.Unlock()
		activeFlapListMu.Unlock()
	}
}

func updateList(cidr string, asPath []bgp.AsPath) {
	if len(asPath) == 0 {
		return
	}
	cleanPath := asPath[0] // Multiple AS paths in a single update message currently unsupported (not used by bird)

	currentTime := time.Now().Unix()
	obj := flapMap[cidr]

	if obj == nil {
		newFlap := &Flap{
			Cidr:                 cidr,
			LastSeen:             currentTime,
			FirstSeen:            currentTime,
			PathChangeCount:      0,
			PathChangeCountTotal: 0,
			LastPath:             make(map[uint32]bgp.AsPath),
		}
		if GlobalKeepPathInfo {
			newFlap.Paths = []bgp.AsPath{cleanPath}
		} else {
			newFlap.Paths = make([]bgp.AsPath, 0, 0)
		}
		newFlap.LastPath[getFirstAsn(cleanPath)] = cleanPath
		flapMap[cidr] = newFlap

		// Notify for every Update
		if NotifyTarget == 0 {
			newFlap.PathChangeCountTotal = 1
			activeFlapListMu.Lock()
			activeFlapList = append(activeFlapList, newFlap)
			activeFlapListMu.Unlock()
			go mainNotify(newFlap)
		}
		return
	}

	// If the entry already exists

	if !pathsEqual(obj.LastPath[getFirstAsn(cleanPath)], cleanPath) {
		if GlobalKeepPathInfo {
			if len(obj.Paths) <= PathLimit {
				exists := false
				for b := range obj.Paths {
					if pathsEqual(obj.Paths[b], cleanPath) {
						exists = true
						break
					}
				}
				if !exists {
					obj.Paths = append(obj.Paths, cleanPath)
				}
			}
		}

		if GlobalPerPeerState {
			if len(obj.LastPath[getFirstAsn(cleanPath)].Asn) == 0 {
				obj.LastPath[getFirstAsn(cleanPath)] = cleanPath
				return
			}
		}

		obj.PathChangeCount = incrementUint64(obj.PathChangeCount)
		obj.PathChangeCountTotal = incrementUint64(obj.PathChangeCountTotal)

		obj.LastSeen = currentTime
		obj.LastPath[getFirstAsn(cleanPath)] = cleanPath

		if obj.PathChangeCount == NotifyTarget {
			if obj.PathChangeCountTotal == NotifyTarget {
				activeFlapListMu.Lock()
				activeFlapList = append(activeFlapList, obj)
				activeFlapListMu.Unlock()
			}
			obj.PathChangeCount = 0
			if GlobalNotifyOnce {
				if obj.PathChangeCountTotal > NotifyTarget {
					return
				}
			}
			go mainNotify(obj)
		}
	}
}

func getFirstAsn(asPath bgp.AsPath) uint32 {
	if GlobalPerPeerState {
		if len(asPath.Asn) == 0 {
			return 0
		}
		return asPath.Asn[0]
	} else {
		return 0
	}
}

func pathsEqual(path1, path2 bgp.AsPath) bool {
	if len(path1.Asn) != len(path2.Asn) {
		return false
	}
	for i := range path1.Asn {
		if path1.Asn[i] != path2.Asn[i] {
			return false
		}
	}
	return true
}

func incrementUint64(n uint64) uint64 {
	if n == math.MaxUint64 {
		return n
	}
	return n + 1
}

var updateDropperRunning int32 = 0

func updateDropper(updateChannel chan *bgp.UserUpdate) {
	log.Println("[WARNING] Can't keep up! Dropping some updates")
	for len(updateChannel) > 10700 {
		for i := 0; i < 50; i++ {
			<-updateChannel
		}
	}
	log.Println("[INFO] Recovered")
	atomic.StoreInt32(&updateDropperRunning, int32(0))
}

func getActiveFlapList() []Flap {
	aFlap := make([]Flap, 0)
	if GlobalDeepCopy {
		flapMapMu.RLock()
		defer flapMapMu.RUnlock()
	}
	activeFlapListMu.RLock()
	defer activeFlapListMu.RUnlock()
	for i := range activeFlapList {
		if GlobalDeepCopy {
			newFlap := &Flap{
				Cidr:                 activeFlapList[i].Cidr,
				FirstSeen:            activeFlapList[i].FirstSeen,
				LastSeen:             activeFlapList[i].LastSeen,
				PathChangeCount:      activeFlapList[i].PathChangeCount,
				PathChangeCountTotal: activeFlapList[i].PathChangeCountTotal,
			}

			newPaths := make([]bgp.AsPath, len(activeFlapList[i].Paths))
			for p := range activeFlapList[i].Paths {
				ASNs := activeFlapList[i].Paths[p].Asn
				newASNs := make([]uint32, len(ASNs))
				copy(newASNs, ASNs)
				newPaths[p] = bgp.AsPath{
					Asn: newASNs,
				}
			}
			newFlap.Paths = newPaths

			newLastPath := make(map[uint32]bgp.AsPath)
			for k, v := range activeFlapList[i].LastPath {
				newLastASNs := make([]uint32, len(v.Asn))
				copy(newLastASNs, v.Asn)
				newLastPath[k] = bgp.AsPath{Asn: newLastASNs}
			}

			newFlap.LastPath = newLastPath
			aFlap = append(aFlap, *newFlap)

		} else {
			aFlap = append(aFlap, *activeFlapList[i])
		}
	}

	return aFlap
}
