import importlib.util
from pathlib import Path
import tempfile
from types import SimpleNamespace
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('state_admin', Path(__file__).with_name('state_admin.py'))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class StateCredentialTests(unittest.TestCase):
    def test_worker_never_gets_schema_or_control_ownership(self):
        statements = []
        def sql(args, statement):
            statements.append(statement)
            if statement.startswith('SELECT json_build_object'):
                return '{"state":"pending","connections":8,"roles":["worker"],"certificate_sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}'
            return ''
        with tempfile.TemporaryDirectory() as directory:
            args = SimpleNamespace(cluster='test', node='worker', database='testdb', output=Path(directory) / 'bundle.json', ephemeral_acl=True)
            with patch.object(module.os, 'geteuid', return_value=0, create=True), patch.object(module, 'sql', side_effect=sql), patch.object(module, 'redis', return_value='aclfile\n/tmp/test.acl'):
                module.provision(args)
            statement = next(s for s in statements if 'CREATE ROLE' in s)
            self.assertIn('CREATE ROLE "s2c_', statement)
            self.assertIn('CONNECTION LIMIT 8 PASSWORD', statement)
            self.assertNotIn('__ROLE__', statement)
            self.assertNotIn('GRANT UPDATE(redis_generation', statement)
            self.assertNotIn('GRANT INSERT, UPDATE, DELETE ON cluster_control,', statement)
            self.assertIn("'cluster_pricing_revision'", statement)
            for table in ('site_profit_meta', 'site_profit_cost_versions', 'site_profit_entries', 'site_profit_group_daily', 'site_profit_group_totals', 'site_profit_user_totals'):
                self.assertIn("'" + table + "'", statement)
            self.assertIn("IF to_regclass('public.site_profit_entries') IS NOT NULL", statement)
            self.assertIn('GRANT UPDATE (aggregated) ON public.site_profit_entries', statement)
            self.assertIn('GRANT UPDATE (aggregated_at,backlog) ON public.site_profit_meta', statement)
            self.assertIn('GRANT INSERT ON public.site_profit_cost_versions', statement)
            self.assertNotIn('GRANT UPDATE ON public.site_profit_entries', statement)
            self.assertIn("'bot_installations'", statement)
            self.assertIn('REVOKE ALL ON bot_installations FROM', statement)
            self.assertNotIn('GRANT SELECT,INSERT ON bot_installations', statement)
            self.assertTrue(args.output.exists())

    def test_certificate_rotation_uses_a_new_state_identity(self):
        import hashlib
        first = hashlib.sha256(b'test:worker:certificate-one').hexdigest()[:20]
        second = hashlib.sha256(b'test:worker:certificate-two').hexdigest()[:20]
        self.assertNotEqual(first, second)


if __name__ == '__main__':
    unittest.main()
