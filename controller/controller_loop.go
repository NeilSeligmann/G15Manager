package controller

import (
	"context"
	"encoding/binary"
	"log"
	"runtime"

	"github.com/NeilSeligmann/G15Manager/system/atkacpi"
	kb "github.com/NeilSeligmann/G15Manager/system/keyboard"
	"github.com/NeilSeligmann/G15Manager/system/plugin"
	"github.com/NeilSeligmann/G15Manager/system/power"
	"github.com/NeilSeligmann/G15Manager/util"

	"github.com/pkg/errors"
)

func (c *Controller) handleACPINotification(haltCtx context.Context) {
	for {
		select {
		case acpi := <-c.acpiCh:
			switch acpi {
			case 87, 88, 207: // ignore these events
				continue
			/*case 87:
				log.Println("acpi: On battery")
			case 88:
				log.Println("acpi: On AC power")
				// this is when you plug in the 180W charger
				// However, plugging in the USB C PD will not show 88 (might need to detect it in user space)
				// there's also this mysterious 207 code, that only pops up when 180W charger is plugged in/unplugged*/
			case 123:
				log.Println("acpi: Power input changed")
				c.workQueueCh[fnCheckCharger].noisy <- false // indicating non initial (continuous) check
			case 233:
				log.Println("acpi: On Lid Open/Close")
			default:
				log.Printf("acpi: Unknown %d\n", acpi)
			}
		case <-haltCtx.Done():
			log.Println("[controller] exiting handleACPINotification")
			return
		}
	}
}

func (c *Controller) handlePowerEvent(haltCtx context.Context) {
	for {
		select {
		case ev := <-c.powerEvCh:
			switch ev {
			case power.PBT_APMRESUMESUSPEND:
				// ignore this event
			case power.PBT_APMSUSPEND:
				log.Println("[controller] housekeeping before suspend")
				c.workQueueCh[fnBeforeSuspend].noisy <- struct{}{}
			case power.PBT_APMRESUMEAUTOMATIC:
				log.Println("[controller] housekeeping after suspend")
				c.workQueueCh[fnAfterSuspend].noisy <- struct{}{}
			}
		case <-haltCtx.Done():
			log.Println("[controller] exiting handlePowerEvent")
			return
		}
	}
}

func (c *Controller) handleKeyPress(haltCtx context.Context) {
	for {
		select {
		case keyCode := <-c.keyCodeCh:
			// log.Printf("Key press: ")
			// log.Println(keyCode)

			switch keyCode {
			case kb.KeyROG:
				log.Println("hid: ROG Key Pressed (debounced)")
				c.workQueueCh[fnUtilityKey].noisy <- struct{}{}

			case kb.KeyFnF5:
				// No longer debounced
				log.Println("hid: Fn + F5 Pressed")
				c.notifyPlugins(plugin.EvtSentinelCycleThermalProfile, int64(1))
				// Old debouncing code
				// log.Println("hid: Fn + F5 Pressed (debounced)")
				// c.workQueueCh[fnThermalProfile].noisy <- struct{}{}
				

			case kb.KeyVolDown:
				log.Println("hid: volume down Pressed")

			case kb.KeyVolUp:
				log.Println("hid: volume up Pressed")

			case kb.KeyFnC:
				log.Println("[controller] request to disable gpu")
				c.notifyPlugins(plugin.EvtSentinelDisableGPU, nil)

			case kb.KeyFnV:
				log.Println("[controller] request to enable gpu")
				c.notifyPlugins(plugin.EvtSentinelEnableGPU, nil)

			case kb.KeyRFKill:
				log.Println("[controller] request to cycle refresh rate")
				c.notifyPlugins(plugin.EvtSentinelCycleRefreshRate, nil)

			case
				kb.KeyLCDUp,
				kb.KeyLCDDown,
				kb.KeySleep:
				c.workQueueCh[fnHwCtrl].noisy <- keyCode

			case
				kb.KeyMuteMic,
				kb.KeyTpadToggle,
				kb.KeyFnF4,
				// kb.KeyFnLeft,
				// kb.KeyFnRight,
				kb.KeyFnUp,
				kb.KeyFnDown:
				c.notifyPlugins(plugin.EvtKeyboardFn, keyCode)

			default:
				log.Printf("hid: Unknown %d\n", keyCode)
			}
		case <-haltCtx.Done():
			log.Println("[controller] exiting handleKeyPress")
			return
		}
	}
}

func (c *Controller) handleWorkQueue(haltCtx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			err := r.(error)
			c.errorCh <- err
		}
	}()

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	for {
		select {
		case ev := <-c.workQueueCh[fnUtilityKey].clean:
			log.Printf("[controller] ROG Key pressed %d times\n", ev.Counter)
			c.notifyPlugins(plugin.EvtSentinelUtilityKey, ev.Counter)

		case ev := <-c.workQueueCh[fnThermalProfile].clean:
			log.Printf("[controller] Fn + F5 pressed %d times\n", ev.Counter)
			c.notifyPlugins(plugin.EvtSentinelCycleThermalProfile, ev.Counter)

		case ev := <-c.workQueueCh[fnCheckCharger].clean:
			function := make([]byte, 4)
			binary.LittleEndian.PutUint32(function, atkacpi.DstsCheckCharger)
			status, err := c.Config.WMI.Evaluate(atkacpi.DSTS, function)
			if err != nil {
				c.errorCh <- errors.New("[controller] cannot check charger status")
				return
			}
			isInitialCheck := ev.Data.(bool)
			switch binary.LittleEndian.Uint32(status[0:4]) {
			case 0x0:
				log.Println("[controller] charger is not plugged in")
				if !isInitialCheck {
					c.workQueueCh[fnAutoThermal].noisy <- chargerUnplugged
				}
			case 0x10001:
				log.Println("[controller] 180W charger plugged in")
				if !isInitialCheck {
					c.workQueueCh[fnAutoThermal].noisy <- chargerPluggedIn
				}
			case 0x10002:
				log.Println("[controller] USB-C PD charger plugged in")
				if !isInitialCheck {
					c.workQueueCh[fnAutoThermal].noisy <- chargerPluggedIn
				}
			}

		case ev := <-c.workQueueCh[fnAutoThermal].clean:
			pluggedInStatus := ev.Data.(chargerStatus)
			if pluggedInStatus == chargerPluggedIn {
				c.notifyPlugins(plugin.EvtChargerPluggedIn, nil)
			} else {
				c.notifyPlugins(plugin.EvtChargerUnplugged, nil)
			}

		case <-c.workQueueCh[fnPersistConfigs].clean:
			if err := c.Config.Registry.Save(); err != nil {
				c.errorCh <- errors.Wrap(err, "[controller] error saving to registry")
				return
			}

		case <-c.workQueueCh[fnApplyConfigs].clean:
			// load configs from registry and try to reapply
			if err := c.Config.Registry.Load(); err != nil {
				c.errorCh <- errors.Wrap(err, "[controller] error loading configurations from registry")
				return
			}
			if err := c.Config.Registry.Apply(); err != nil {
				c.errorCh <- errors.Wrap(err, "[controller] error applying configurations")
				return
			}

		case <-c.workQueueCh[fnBroadcastClients].clean:
			c.Config.Registry.ClientCallback()

		case ev := <-c.workQueueCh[fnHwCtrl].clean:
			keyCode := ev.Data.(uint32)
			args := make([]byte, 8)
			log.Printf("hwCtrl: notification from keypress on %d\n", keyCode)

			binary.LittleEndian.PutUint32(args[0:], atkacpi.DevsHardwareCtrl)
			binary.LittleEndian.PutUint32(args[4:], keyCode)

			_, err := c.Config.WMI.Evaluate(atkacpi.DEVS, args)
			if err != nil {
				c.errorCh <- errors.Wrap(err, "hwCtrl: error sending key code to ATKACPI")
				return
			}

		case <-c.workQueueCh[fnBeforeSuspend].clean:
			c.notifyPlugins(plugin.EvtACPISuspend, nil)

		case <-c.workQueueCh[fnAfterSuspend].clean:
			log.Println("[controller] re-apply config")
			c.notifyPlugins(plugin.EvtACPIResume, nil)
			c.workQueueCh[fnApplyConfigs].noisy <- struct{}{}

		case <-haltCtx.Done():
			log.Println("[controller] exiting handleWorkQueue")
			return
		}
	}
}

func (c *Controller) notifyPlugins(evt plugin.Event, val interface{}) {
	t := plugin.Notification{
		Event: evt,
		Value: val,
	}
	for _, p := range c.Config.Plugins {
		go p.Notify(t)
	}
}

func (c *Controller) handlePluginCallback(haltCtx context.Context) {
	for {
		select {
		case t := <-c.pluginCbCh:
			switch t.Event {
			case plugin.CbPersistConfig:
				c.workQueueCh[fnPersistConfigs].noisy <- struct{}{}
				c.workQueueCh[fnBroadcastClients].noisy <- struct{}{}
			case plugin.CbNotifyClients:
				c.workQueueCh[fnBroadcastClients].noisy <- struct{}{}
			case plugin.CbNotifyToast:
				if n, ok := t.Value.(util.Notification); ok {
					c.Config.Notifier <- n
				}
			}
		case <-haltCtx.Done():
			log.Println("[controller] exiting handlePluginCallback")
			return
		}
	}
}
