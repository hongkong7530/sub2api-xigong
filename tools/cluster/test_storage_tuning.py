import contextlib
import importlib.util
import io
from pathlib import Path
from types import SimpleNamespace
import tempfile
import unittest

spec = importlib.util.spec_from_file_location('storage_tuning', Path(__file__).with_name('storage_tuning.py'))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class StorageTuningTests(unittest.TestCase):
    def test_profile_preserves_durability_and_requires_retention_approval(self):
        with tempfile.TemporaryDirectory() as directory:
            args = SimpleNamespace(service='sub2api-worker', directory=Path(directory) / 'review', db_connections=64, cluster_db_budget=224, max_inflight=32, max_queue=8)
            with contextlib.redirect_stdout(io.StringIO()):
                module.generate(args)
            config = (args.directory / 'service.conf').read_text()
            self.assertIn('CLUSTER_RETENTION_APPROVED=false', config)
            self.assertIn('DATABASE_MAX_OPEN_CONNS=64', config)
            self.assertIn('LOG_OUTPUT_TO_STDOUT=false', config)
            self.assertIn('LOG_ROTATION_MAX_BACKUPS=4', config)
            self.assertNotIn('MemoryMax', config)
            self.assertNotIn('fsync', config)
            with self.assertRaises(ValueError):
                module.generate(args)

    def test_invalid_budget_cannot_generate_configuration(self):
        with tempfile.TemporaryDirectory() as directory:
            args = SimpleNamespace(service='worker', directory=Path(directory) / 'review', db_connections=300, cluster_db_budget=224, max_inflight=32, max_queue=8)
            with self.assertRaises(ValueError):
                module.generate(args)
            self.assertFalse(args.directory.exists())


if __name__ == '__main__':
    unittest.main()
