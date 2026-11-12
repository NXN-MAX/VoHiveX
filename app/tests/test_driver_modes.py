"""Exercise driver layout checks against temporary sysfs fixtures only."""
import os
from pathlib import Path
import subprocess
import tempfile
import unittest


APP = Path(__file__).resolve().parents[1]


class DriverModes(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.sys = Path(self.tmp.name) / 'sys'
        self.usb = self.sys / 'bus/usb/devices'
        self.usb.mkdir(parents=True)
        for module in ('option', 'qmi_wwan', 'cdc_ether'):
            (self.sys / 'module' / module).mkdir(parents=True)
        for path in ('bus/usb-serial/drivers/option1/new_id', 'bus/usb/drivers/qmi_wwan/new_id'):
            node = self.sys / path
            node.parent.mkdir(parents=True)
            node.touch()
        self.device = self.usb / '1-1'
        self.device.mkdir()
        for name, value in [('idVendor', '2ca3'), ('idProduct', '4006'), ('bConfigurationValue', '1')]:
            (self.device / name).write_text(value)
        self.interface(2, 'ff', '00', 'option')
        node = self.usb / '1-1:1.2/tty/ttyUSB0'
        node.parent.mkdir()
        node.touch()

    def interface(self, number, cls, subclass, driver):
        path = self.usb / f'1-1:1.{number}'
        path.mkdir(exist_ok=True)
        for name, value in [('bInterfaceClass', cls), ('bInterfaceSubClass', subclass)]:
            (path / name).write_text(value)
        link = path / 'driver'
        if link.is_symlink():
            link.unlink()
        link.symlink_to('/fixture/drivers/' + driver)
        return path

    def qmi(self):
        path = self.interface(4, 'ff', 'ff', 'qmi_wwan') / 'usbmisc/cdc-wdm0'
        path.parent.mkdir()
        path.touch()

    def ecm(self):
        path = self.interface(4, '02', '06', 'cdc_ether')
        (path / 'net/wwan0').mkdir(parents=True)
        self.interface(5, '0a', '00', 'cdc_ether')

    def check_driver(self):
        result = subprocess.run(
            ['/bin/sh', '-c', '. "$1"; validate_devices; mkdir -p "$STATE"; reconcile; health',
             'fixture-test', str(APP / 'driver-lib.sh')],
            env={**os.environ, 'QMI_SYS_ROOT': str(self.sys),
                 'QMI_STATE_ROOT': str(Path(self.tmp.name) / 'state')},
            text=True, capture_output=True,
        )
        return result

    def test_qmi_layout_remains_healthy(self):
        self.qmi()
        result = self.check_driver()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_ecm_keeps_cdc_ether_and_accepts_network_node(self):
        self.ecm()
        result = self.check_driver()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual((self.usb / '1-1:1.4/driver').readlink().name, 'cdc_ether')

    def test_ecm_does_not_require_or_register_qmi(self):
        self.ecm()
        (self.sys / 'module/qmi_wwan').rmdir()
        (self.sys / 'bus/usb/drivers/qmi_wwan/new_id').unlink()
        result = self.check_driver()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertFalse((Path(self.tmp.name) / 'state/qmi-id').exists())

    def test_empty_host_starts_without_kernel_modules(self):
        for p in self.device.iterdir():
            p.unlink()
        result = subprocess.run(
            ['/bin/sh', '-c', '. "$1"; mkdir -p "$STATE"; load_modules; register_ids',
             'fixture-test', str(APP / 'driver-lib.sh')],
            env={**os.environ, 'QMI_SYS_ROOT': str(self.sys),
                 'QMI_STATE_ROOT': str(Path(self.tmp.name) / 'state')},
            text=True, capture_output=True,
        )
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertFalse((Path(self.tmp.name) / 'state/option-id').exists())

    def test_repeated_reconciliation_reuses_registration(self):
        self.ecm()
        self.assertEqual(self.check_driver().returncode, 0)
        (self.sys / 'bus/usb-serial/drivers/option1/new_id').unlink()
        result = self.check_driver()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_unknown_layout_is_rejected(self):
        self.interface(4, '03', '00', 'none')
        self.assertNotEqual(self.check_driver().returncode, 0)

    def test_ecm_foreign_driver_is_rejected(self):
        self.ecm()
        self.interface(4, '02', '06', 'rndis_host')
        self.assertNotEqual(self.check_driver().returncode, 0)

    def test_ecm_without_network_node_is_unhealthy(self):
        self.ecm()
        (self.usb / '1-1:1.4/net/wwan0').rmdir()
        self.assertNotEqual(self.check_driver().returncode, 0)

    def test_qmi_without_control_node_is_unhealthy(self):
        self.qmi()
        (self.usb / '1-1:1.4/usbmisc/cdc-wdm0').unlink()
        self.assertNotEqual(self.check_driver().returncode, 0)


if __name__ == '__main__':
    unittest.main()
