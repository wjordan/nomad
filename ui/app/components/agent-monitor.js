import { inject as service } from '@ember/service';
import Component from '@ember/component';
import { computed } from '@ember/object';
import { assert } from '@ember/debug';
import { tagName } from '@ember-decorators/component';
import Log from 'nomad-ui/utils/classes/log';

const LEVELS = ['error', 'warn', 'info', 'debug', 'trace'];

@tagName('')
export default class AgentMonitor extends Component {
  @service token;

  client = null;
  server = null;
  level = LEVELS[2];
  onLevelChange() {}

  levels = LEVELS;
  monitorUrl = '/v1/agent/monitor';
  isStreaming = true;
  logger = null;

  @computed('level', 'client.id', 'server.id')
  get monitorParams() {
    assert(
      'Provide a client OR a server to AgentMonitor, not both.',
      this.server != null || this.client != null
    );

    const type = this.server ? 'server_id' : 'client_id';
    const id = this.server ? this.server.id : this.client && this.client.id;

    return {
      log_level: this.level,
      [type]: id,
    };
  }

  didInsertElement() {
    this.updateLogger();
  }

  updateLogger() {
    let currentTail = this.logger ? this.logger.tail : '';
    if (currentTail) {
      currentTail += `\n...changing log level to ${this.level}...\n\n`;
    }
    this.set(
      'logger',
      Log.create({
        logFetch: url => this.token.authorizedRequest(url),
        params: this.monitorParams,
        url: this.monitorUrl,
        tail: currentTail,
      })
    );
  }

  setLevel(level) {
    this.logger.stop();
    this.set('level', level);
    this.onLevelChange(level);
    this.updateLogger();
  }

  toggleStream() {
    this.set('streamMode', 'streaming');
    this.toggleProperty('isStreaming');
  }
}
